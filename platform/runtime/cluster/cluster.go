package cluster

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/runtime/router"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/logging"
)

// directiveRequestKey names a request of an engine to its cluster: the engine and the number it
// gave it.
type directiveRequestKey struct {
	from      sim.StateID
	requestID uint64
}

// pendingRetirement is a request to end an engine, which waits for the engine to close.
type pendingRetirement struct {
	from    sim.StateID
	request sim.DirectiveRequest
	engine  engine.EngineHandle
}

// Cluster is the engines of a game, wired. It knows only that they are engines: what each does is
// the business of its state. The first of them is the coordinator engine, which every cluster
// has; the others are made by the requests of the engines, among them the commands that the code
// that runs the cluster sends it (see RunPeriodicTasks).
type Cluster struct {
	Env               *engine.Environment
	coordinatorEngine *engine.Engine[*sim.CoordinatorState]

	// mu protects the list of engines and enginesToRun: the cluster drops an engine from the list
	// when it has ended, on the goroutine that ran it.
	mu      sync.RWMutex
	engines []engine.EngineHandle

	// enginesToRun are the engines that Run has not taken yet, every engine that is added (before
	// Run or while it runs), and wakeRun wakes Run when one is added.
	enginesToRun []engine.EngineHandle
	wakeRun      chan struct{}

	// pendingCreations are the engines that a request made, whose creator has not yet taken the
	// response: they are outside the router and run by no goroutine. pendingRetirements are the
	// requests to end an engine that wait for it to close. Only the periodic task uses them.
	pendingCreations   map[directiveRequestKey]engine.EngineHandle
	pendingRetirements []pendingRetirement

	// orchestrator gives the IDs of the engines that the cluster makes and brings the
	// configuration of the world into the coordinator engine; nil for a replay, which does
	// neither
	orchestrator *Orchestrator
}

// NewCluster makes a cluster that runs, in the environment: it has its coordinator engine and no
// other, made from the checkpoint given or, when it is nil, new with every variable at its default.
// The environment gets a directive router when it has none. The environment's codecs and engine
// registry must know the coordinator engine (see NewCodecs and NewEngineRegistry). The orchestrator
// gives the IDs of the engines that the cluster makes, and holds the configuration of the world
// outside the game, which it sends to the coordinator engine when it changes.
func NewCluster(
	env *engine.Environment, orchestrator *Orchestrator, coordinatorCheckpoint []byte,
) (*Cluster, error) {
	if orchestrator == nil {
		return nil, errors.New("a cluster that runs needs an orchestrator")
	}
	if env.DirectiveRouter == nil {
		env.DirectiveRouter = router.NewDirectiveRouter()
	}
	c, err := buildCluster(env, coordinatorCheckpoint)
	if err != nil {
		return nil, err
	}
	orchestrator.beginConfigSync(c.coordinatorEngine.State().Snapshot())
	c.orchestrator = orchestrator
	return c, nil
}

// NewReplayCluster makes the cluster of a replay: as NewCluster does, but it makes no engine, since
// the log says which engines were made and retired and when, and what the cluster answered. The
// environment has the directive router that the replay gives the responses of the log to.
func NewReplayCluster(env *engine.Environment, coordinatorCheckpoint []byte) (*Cluster, error) {
	return buildCluster(env, coordinatorCheckpoint)
}

func buildCluster(env *engine.Environment, coordinatorCheckpoint []byte) (*Cluster, error) {
	var coordinatorEngine *engine.Engine[*sim.CoordinatorState]
	if coordinatorCheckpoint == nil {
		coordinatorEngine = engine.NewEngine(sim.NewCoordinatorState(), sim.CoordinatorStateID, env)
	} else {
		decoded, err := engine.DecodeCheckpoint[*sim.CoordinatorState](
			coordinatorCheckpoint, env.Codecs,
		)
		if err != nil {
			return nil, fmt.Errorf("the coordinator engine: %w", err)
		}
		if coordinatorEngine, err = decoded.Restore(env); err != nil {
			return nil, fmt.Errorf("the coordinator engine: %w", err)
		}
	}
	c := &Cluster{
		Env: env, coordinatorEngine: coordinatorEngine, wakeRun: make(chan struct{}, 1),
		pendingCreations: map[directiveRequestKey]engine.EngineHandle{},
	}
	c.addEngineLocked(coordinatorEngine)
	return c, nil
}

// Forget drops an engine from the list of the cluster. It is for an engine that has retired: it is
// not retired by this, and the router still knows it. Run calls it for the engines it runs, and the
// code that steps the engines itself calls it for the ones that retired between two steps.
func (c *Cluster) Forget(id sim.StateID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.engines = slices.DeleteFunc(
		c.engines, func(e engine.EngineHandle) bool { return e.StateID() == id },
	)
}

// RunPeriodicTasks does what the cluster does at every period, from one goroutine at a time: it
// sends the coordinator engine what the store holds, takes the engines that were made by a
// request into the cluster when their creators have taken the response, drops the ones whose
// creator is gone, serves the requests that the engines published, and answers the requests to end
// an engine whose engine has closed. `Run` calls it on a ticker; the code that steps the engines
// itself calls it between two steps.
func (c *Cluster) RunPeriodicTasks() {
	c.orchestrator.SyncConfig(c.coordinatorEngine)
	c.adoptAcknowledgedEngines()
	c.retireEnginesWhoseCreatorIsGone()
	c.serveDirectiveRequests()
	c.answerFinishedRetirements()
}

// RequestEngineRetirement asks the engine to end itself, between two of its ticks: on its own
// goroutine when Run runs it, and when the code that steps the engines retires it otherwise
// (stepping.Step). It does not wait, and may be called from any goroutine. It reports whether the
// cluster has the engine. It is for the code that runs the cluster: the engines ask by a request.
func (c *Cluster) RequestEngineRetirement(id sim.StateID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, e := range c.engines {
		if e.StateID() == id {
			e.RequestRetirement(0, 0)
			return true
		}
	}
	return false
}

// Orchestrator is what gives the IDs of the engines that the cluster makes; nil for a replay.
func (c *Cluster) Orchestrator() *Orchestrator { return c.orchestrator }

// AddEngineByCheckpoint makes the engine of a checkpoint, at its tick, and adds it to the cluster:
// a replay and a recovery make again the engines that they replay from the checkpoint they start
// from, or from the one inside the record that begins the log of an engine that was made new.
func (c *Cluster) AddEngineByCheckpoint(checkpoint []byte) (engine.EngineHandle, error) {
	e, err := c.Env.EngineRegistry.Restore(c.Env, checkpoint)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addEngineLocked(e)
	return e, nil
}

// ConfigStore is the configuration of the world outside the game, or nil.
func (c *Cluster) ConfigStore() *config.Store {
	if c.orchestrator == nil {
		return nil
	}
	return c.orchestrator.ConfigStore()
}

// CoordinatorEngine returns the coordinator engine of the cluster: the one to send an
// UpdateConfigCommand to.
func (c *Cluster) CoordinatorEngine() *engine.Engine[*sim.CoordinatorState] {
	return c.coordinatorEngine
}

// Engines returns the engines, in the order they were added.
func (c *Cluster) Engines() []engine.EngineHandle {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return slices.Clone(c.engines)
}

// Find returns the engine with the ID, whatever its state is.
func (c *Cluster) Find(id sim.StateID) (engine.EngineHandle, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, e := range c.engines {
		if e.StateID() == id {
			return e, true
		}
	}
	return nil, false
}

// FindOfType returns the engine with the ID as an engine of a state of type S, or false when there
// is none
// or its state is of another type.
func (c *Cluster) FindOfType[S sim.State](id sim.StateID) (*engine.Engine[S], bool) {
	e, ok := c.Find(id)
	if !ok {
		return nil, false
	}
	typed, ok := e.(*engine.Engine[S])
	return typed, ok
}

// AllOfType returns the engines whose state is of type S, in the order they were added.
func (c *Cluster) AllOfType[S sim.State]() []*engine.Engine[S] {
	var out []*engine.Engine[S]
	for _, e := range c.Engines() {
		if typed, ok := e.(*engine.Engine[S]); ok {
			out = append(out, typed)
		}
	}
	return out
}

// Run runs every engine of the cluster on a goroutine of its own, ticking every period, the ones
// that are added while it runs included, and returns when the context ends and all of them have
// stopped. An engine is not waited for by the others. When the tick of an engine fails, the whole
// cluster stops as if the context had ended, and the error is returned. A cluster of a replay has
// no orchestrator and does not run: Run returns an error at once. An engine that an action asked
// to retire ends between two of its ticks on its own goroutine, and the cluster drops it.
func (c *Cluster) Run(ctx context.Context, period time.Duration) error {
	if c.orchestrator == nil {
		return errors.New("a cluster of a replay has no orchestrator and does not run")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		wg       sync.WaitGroup
		failOnce sync.Once
		failure  error
	)
	runWaitingEngines := func() {
		c.mu.Lock()
		waiting := c.enginesToRun
		c.enginesToRun = nil
		c.mu.Unlock()
		for _, e := range waiting {
			if _, retired := e.IsRetired(); retired { // it retired before Run took it
				continue
			}
			wg.Go(func() {
				t := time.NewTicker(period)
				defer t.Stop()
				if err := e.Run(ctx, t.C); err != nil {
					failOnce.Do(func() {
						failure = err
						cancel()
					})
				}
				if _, retired := e.IsRetired(); retired {
					c.Forget(e.StateID())
				}
			})
		}
	}
	runWaitingEngines()
	wg.Go(func() {
		t := time.NewTicker(period)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.wakeRun:
				runWaitingEngines()
				continue
			case <-c.orchestrator.ConfigChanged():
			case <-t.C: // a command that the engine could not take is sent again
			}
			c.RunPeriodicTasks()
		}
	})
	<-ctx.Done()
	wg.Wait()
	c.Env.Router.Stop() // after the engines: each runs its last tick with the router open
	if failure != nil {
		return fmt.Errorf("the cluster stopped: %w", failure)
	}
	return nil
}

// addEngineLocked puts an engine in the list of the cluster and among the ones that Run is to run.
// The caller holds mu.
func (c *Cluster) addEngineLocked(e engine.EngineHandle) {
	c.engines = append(c.engines, e)
	c.enginesToRun = append(c.enginesToRun, e)
	select {
	case c.wakeRun <- struct{}{}:
	default: // Run is woken already
	}
}

// adoptAcknowledgedEngines takes into the cluster the engines whose creators took the response to
// the request
// that made them: they join the router and, from then on, run as every engine does.
func (c *Cluster) adoptAcknowledgedEngines() {
	for _, a := range c.Env.DirectiveRouter.TakeResponseAcknowledgements() {
		key := directiveRequestKey{a.From, a.Request}
		handle, found := c.pendingCreations[key]
		if !found {
			logging.Warn(
				"datafix: an acknowledgement is for an engine that is not waiting",
				"engine", a.From, "request", a.Request,
			)
			continue
		}
		delete(c.pendingCreations, key)
		handle.Adopt(a.From, a.Tick)
		c.mu.Lock()
		c.addEngineLocked(handle)
		c.mu.Unlock()
	}
}

// retireEnginesWhoseCreatorIsGone ends the engines that wait for their creators to take the
// response
// when the creator is gone: no one will, and the engine never ran, so nothing depends on it. A
// creator that took the response and then left published its acknowledgement before, and
// adoptAcknowledgedEngines, which runs first, took the engine in.
func (c *Cluster) retireEnginesWhoseCreatorIsGone() {
	for key, handle := range c.pendingCreations {
		if creator, found := c.Find(key.from); found {
			if _, retired := creator.IsRetired(); !retired {
				continue
			}
		}
		delete(c.pendingCreations, key)
		handle.Retire()
	}
}

// serveDirectiveRequests serves the requests that the engines published, in the order they were
// published, and answers each to the engine that asked.
func (c *Cluster) serveDirectiveRequests() {
	for _, published := range c.Env.DirectiveRouter.TakeRequests() {
		from, request := published.From, published.Request
		response := sim.DirectiveResponse{ID: request.ID, Kind: request.Kind, Tag: request.Tag}
		switch request.Kind {
		case sim.DirectiveCreateEngine:
			handle, err := c.makeRequestedEngine(
				from, request.Tick, request.ID, request.State.Clone(), request.StartActions,
			)
			if err != nil {
				logging.Error("the engine could not be made", "asker", from, "err", err)
				response.Failure = err.Error()
				break
			}
			response.Engine = handle.StateID()
			c.pendingCreations[directiveRequestKey{from, request.ID}] = handle
		case sim.DirectiveRetireEngine:
			if !c.serveRetireEngineRequest(from, request, &response) {
				continue
			}
		default:
			response.Failure = fmt.Sprintf(
				"the kind %d of request is not one the cluster serves", request.Kind,
			)
		}
		if c.Env.DirectiveRouter.PublishResponse(from, response) {
			continue
		}
		// the engine that asked has left: no one takes the response
		if handle, found := c.pendingCreations[directiveRequestKey{from, request.ID}]; found {
			delete(c.pendingCreations, directiveRequestKey{from, request.ID})
			handle.Retire()
		}
	}
}

// serveRetireEngineRequest asks the engine that a request names to retire, and keeps the request
// until the
// engine has closed. It reports whether the request has a response now, which it puts in response;
// an engine that asked for itself to end has none: it is gone.
func (c *Cluster) serveRetireEngineRequest(
	from sim.StateID, request sim.DirectiveRequest, response *sim.DirectiveResponse,
) bool {
	response.Engine = request.Target
	if request.Target == sim.CoordinatorStateID {
		response.Failure = "the coordinator engine cannot be ended"
		return true
	}
	handle, found := c.Find(request.Target)
	if !found {
		response.Failure = "the engine is not there"
		return true
	}
	if request.Target == from {
		handle.RequestRetirement(0, 0)
		return false
	}
	handle.RequestRetirement(from, request.Tick)
	c.pendingRetirements = append(c.pendingRetirements, pendingRetirement{from, request, handle})
	return false
}

// answerFinishedRetirements answers the requests to end an engine whose engine has closed, with the
// tick
// of the last frame of its log, which the answer depends on.
func (c *Cluster) answerFinishedRetirements() {
	waiting := c.pendingRetirements[:0]
	for _, p := range c.pendingRetirements {
		retiredTick, retired := p.engine.IsRetired()
		if !retired {
			waiting = append(waiting, p)
			continue
		}
		c.Env.DirectiveRouter.PublishResponse(p.from, sim.DirectiveResponse{
			ID: p.request.ID, Kind: p.request.Kind, Tag: p.request.Tag,
			Engine: p.request.Target, Tick: retiredTick,
		})
	}
	c.pendingRetirements = waiting
}

// makeRequestedEngine makes an engine of the state, with the ID that the orchestrator gives, and
// runs its
// first tick: the record of how it was made is in its log. The engine is outside the router, and
// run by no goroutine, until its creator has taken the response (Engine.Adopt).
func (c *Cluster) makeRequestedEngine(
	creator sim.StateID, creatorTick, directiveID uint64, st sim.State,
	startActions []sim.BaseAction,
) (engine.EngineHandle, error) {
	if c.Env.EngineRegistry == nil {
		return nil, errors.New("the cluster has no registry of engines")
	}
	id, err := c.orchestrator.NextStateID(st.Type())
	if err != nil {
		return nil, err
	}
	return c.Env.EngineRegistry.Make(c.Env, id, st, startActions, creator, creatorTick, directiveID)
}
