package engine

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/logging"
)

// Environment is what every engine of a cluster shares: the router that carries their messages, the
// event log they write, the store of the states they publish (the configuration is read from it,
// from the state of the configuration engine), the emitter where they give what changed, the codecs
// that read back what they wrote, the registry that makes their engines, the seed of their random
// numbers, and the writer of their checkpoints.
type Environment struct {
	Router         MessageRouter
	EventLog       eventlog.Log
	StableStore    StableStore
	Codecs         *sim.Codecs
	EngineRegistry *EngineRegistry
	Seed           uint64
	QueueSize      int
	Emitter        Emitter
	// AckBoard is where the engines say what they hold safe and read what their receivers hold. An
	// engine with none holds no message for an ack.
	AckBoard *AckBoard
	// CheckpointWriter writes the checkpoints that the engines take at the end of a tick. With
	// none the engines take none: a replay has none.
	CheckpointWriter CheckpointWriter
	// Directives is where the engines hand the requests of their actions to make and to remove
	// engines: NewCluster sets it to the cluster. A replay has none, and its engines ignore the
	// requests.
	Directives DirectiveSink
}

// EngineHandle is what the cluster needs of an engine, whatever the type of its state is: to run
// it on its own, to end it, to give it a response of the cluster and to say which engine it is.
// What the code that holds an engine from the cluster (Cluster.Engines, Cluster.Find,
// Cluster.AddEngineByCheckpoint) needs besides, it asks of the handle itself: it defines its own
// interface and converts the handle to it.
type EngineHandle interface {
	StateID() sim.StateID
	// Run runs the engine on its own until the context ends or its tick fails.
	Run(ctx context.Context, ticks <-chan time.Time) error
	// Retire ends the engine for good and writes the record of it to its log: see Engine.Retire.
	Retire()
	// HandleDirectiveResponse gives the engine the response of the cluster to a request of it: see
	// Engine.HandleDirectiveResponse.
	HandleDirectiveResponse(d sim.DirectiveResponse) error
	// RequestRetirement asks the engine to retire between two of its ticks: Run does it, or the
	// code that steps the engines itself (Engine.RetireIfRequested); it does not wait, and may be
	// called from any goroutine.
	RequestRetirement()
	// IsClosed says that the engine has been closed: it retired or was closed.
	IsClosed() bool
}

// DirectiveKey names a request of an engine to its cluster: the engine and the number it gave it.
type DirectiveKey struct {
	Engine sim.StateID
	ID     uint64
}

// Cluster is the engines of a game, wired. It knows only that they are engines: what each does is
// the business of its state. The first of them is the configuration engine, which every cluster
// has; the others are added by the code that runs the cluster (CreateEngine) and made by the
// requests of the engines (see DirectiveSink).
type Cluster struct {
	Env    *Environment
	config *Engine[*sim.ConfigState]

	// mu protects the list of engines, enginesToRun, running, the IDs and served: an engine that
	// runs on its own goroutine can ask for another to be made or removed, and the cluster drops
	// an engine from the list when it has ended.
	mu      sync.RWMutex
	engines []EngineHandle
	// enginesToRun are the engines that Run has not taken yet, every engine that is added (before
	// Run or while it runs), and wakeRun wakes Run when one is added.
	enginesToRun []EngineHandle
	wakeRun      chan struct{}
	// running says that Run runs the engines on their own goroutines: a removal is then asked of
	// the engine, which retires between two of its ticks.
	running bool
	// ids are the highest numbers that were given, for each type of state
	ids map[sim.StateType]uint64
	// served says which engine each request that made one was served with, so that a request
	// that is handed over again gets the answer again and not another engine
	served map[DirectiveKey]sim.StateID

	// store is the configuration of the world outside the game, which the cluster brings into the
	// configuration engine; nil means none. syncedConfig is the configuration that the engine has,
	// or is sent, from it, and syncedConfigGeneration the generation of the document that made it.
	store                  *config.Store
	syncedConfig           config.ReadOnlySnapshot
	syncedConfigGeneration uint64
}

var _ DirectiveSink = (*Cluster)(nil)

// NewCluster makes a cluster that runs, in the environment: it has its configuration engine and no
// other, made from the checkpoint given or, when it is nil, new with every variable at its default.
// The cluster becomes where the engines hand their requests to (Environment.Directives). The
// environment's codecs and engine registry must know the configuration engine (see NewCodecs and
// NewEngineRegistry). The store is the configuration of the world outside the game, which the
// cluster sends to the configuration engine when it changes; nil means none.
func NewCluster(
	env *Environment, store *config.Store, configCheckpoint []byte,
) (*Cluster, error) {
	c, err := buildCluster(env, store, configCheckpoint)
	if err != nil {
		return nil, err
	}
	env.Directives = c
	return c, nil
}

// NewReplayCluster makes the cluster of a replay: as NewCluster does, but the engines hand their
// requests to no one, since the log says which engines were made and removed and when, and what the
// cluster answered.
func NewReplayCluster(env *Environment, configCheckpoint []byte) (*Cluster, error) {
	env.Directives = nil
	return buildCluster(env, nil, configCheckpoint)
}

func buildCluster(
	env *Environment, store *config.Store, configCheckpoint []byte,
) (*Cluster, error) {
	var configEngine *Engine[*sim.ConfigState]
	if configCheckpoint == nil {
		configEngine = NewEngine(sim.NewConfigState(), sim.ConfigStateID, env)
	} else {
		decoded, err := DecodeEngineCheckpoint[*sim.ConfigState](configCheckpoint, env.Codecs)
		if err != nil {
			return nil, fmt.Errorf("the configuration engine: %w", err)
		}
		if configEngine, err = decoded.Restore(env); err != nil {
			return nil, fmt.Errorf("the configuration engine: %w", err)
		}
	}
	c := &Cluster{
		Env: env, config: configEngine, wakeRun: make(chan struct{}, 1),
		ids:    map[sim.StateType]uint64{},
		served: map[DirectiveKey]sim.StateID{}, store: store,
		syncedConfig: configEngine.State().Snapshot(),
	}
	c.addEngineLocked(configEngine)
	return c, nil
}

// addEngineLocked puts an engine in the list of the cluster and among the ones that Run is to run.
// The caller holds mu.
func (c *Cluster) addEngineLocked(e EngineHandle) {
	c.engines = append(c.engines, e)
	c.enginesToRun = append(c.enginesToRun, e)
	select {
	case c.wakeRun <- struct{}{}:
	default: // Run is woken already
	}
}

// RemoveEngine retires the engine with the state ID for good, and forgets it: see Engine.Retire. It
// reports whether the cluster had one. The configuration engine is never removed: every engine
// reads its configuration from it. While the cluster runs its engines on their own goroutines the
// engine retires between two of its ticks, so RemoveEngine only asks it to.
func (c *Cluster) RemoveEngine(id sim.StateID) bool {
	if id == sim.ConfigStateID {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, e := range c.engines {
		if e.StateID() != id {
			continue
		}
		if c.running {
			e.RequestRetirement()
		} else {
			e.Retire()
			c.engines = slices.Delete(c.engines, i, i+1)
		}
		return true
	}
	return false
}

// requestEngineRemoval asks the engine to end itself, between two of its ticks: on its own
// goroutine when Run runs it, and when the code that steps the engines retires it otherwise. It
// does not wait, and may be called from the tick of an engine, which RemoveEngine may not. It
// reports whether the cluster has the engine.
func (c *Cluster) requestEngineRemoval(id sim.StateID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, e := range c.engines {
		if e.StateID() == id {
			e.RequestRetirement()
			return true
		}
	}
	return false
}

// Forget drops an engine from the list of the cluster. It is for an engine that has retired: it is
// not retired by this, and the router still knows it. Run calls it for the engines it runs, and the
// code that steps the engines itself calls it for the ones that retired between two steps.
func (c *Cluster) Forget(id sim.StateID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.engines = slices.DeleteFunc(c.engines, func(e EngineHandle) bool { return e.StateID() == id })
}

// NewStateID gives the next ID of a type of state: the numbers of a type are counted up from 1 and
// never given again, so an ID names one thing for good, in a log, in a checkpoint and in a message
// that waits. The cluster gives the IDs of all the engines that it makes.
func (c *Cluster) NewStateID(t sim.StateType) sim.StateID {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nextStateIDLocked(t)
}

func (c *Cluster) nextStateIDLocked(t sim.StateType) sim.StateID {
	c.ids[t]++
	return sim.NewStateID(t, c.ids[t])
}

// RaiseStateIDs makes the next IDs of the types higher than the numbers given, which are the
// highest that were given before the cluster stopped (the logs of the engines say them: replay.Plan
// has them): an ID is never given again, whether the engine that had it left a trace or not.
func (c *Cluster) RaiseStateIDs(last map[sim.StateType]uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for t, n := range last {
		c.ids[t] = max(c.ids[t], n)
	}
}

// RestoreServed tells the cluster which engine each request that made one was served with, as the
// logs of the engines say (the record that begins each says which request it was made for): a
// request that an engine hands over again after a recovery gets its answer again and is not served
// twice.
func (c *Cluster) RestoreServed(served map[DirectiveKey]sim.StateID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	maps.Copy(c.served, served)
}

// CreateEngine makes an engine of the state, with the actions it starts with, and adds it to the
// cluster after the engines that are in it: it is for the code that runs the cluster, as the
// requests of the engines are for the engines. The state has no ID: the cluster gives the engine
// one, which it returns. The engine is new, at the tick of its state, and registered with the
// router; its log begins with a record that holds it as it is. When `Run` runs the engines it runs
// this one too, on a goroutine of its own.
func (c *Cluster) CreateEngine(
	st sim.State, start ...sim.BaseAction,
) (sim.StateID, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.makeEngineLocked(0, 0, 0, st, start)
}

// AddEngineByCheckpoint makes the engine of a checkpoint, at its tick, and adds it to the cluster:
// a replay and a recovery make again the engines that they replay from the checkpoint they start
// from, or from the one inside the record that begins the log of an engine that was made new.
func (c *Cluster) AddEngineByCheckpoint(checkpoint []byte) (EngineHandle, error) {
	e, err := c.Env.EngineRegistry.Restore(c.Env, checkpoint)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addEngineLocked(e)
	return e, nil
}

// ServeDirectives serves the requests that an engine hands its cluster when the tick of its
// actions is done: an engine is made now, so that a message of the tick to a new engine finds it,
// and the engine that is to be removed ends between two of its ticks. Each request is answered to
// the engine that asked.
func (c *Cluster) ServeDirectives(
	from sim.StateID, tick uint64, requests []sim.DirectiveRequest,
) {
	for _, request := range requests {
		response := sim.DirectiveResponse{ID: request.ID, Kind: request.Kind, Tag: request.Tag}
		switch request.Kind {
		case sim.DirectiveCreateEngine:
			c.mu.Lock()
			id, err := c.createEngineLocked(from, tick, request)
			c.mu.Unlock()
			response.Engine = id
			if err != nil {
				logging.Error("the engine could not be made", "asker", from, "err", err)
				response.Failure = err.Error()
			}
		case sim.DirectiveRemoveEngine:
			response.Engine = request.Target
			if request.Target == sim.ConfigStateID {
				response.Failure = "the configuration engine cannot be removed"
			} else {
				c.requestEngineRemoval(request.Target)
			}
		default:
			response.Failure = fmt.Sprintf(
				"the kind %d of request is not one the cluster serves", request.Kind,
			)
		}
		c.sendDirectiveResponse(from, response)
	}
}

// sendDirectiveResponse gives the engine the response to its request. A response that cannot be
// given now (the queue of the engine is full) comes again with the next hand-over of the request;
// one for an engine that is gone is lost, and the log says so in a line that begins with
// "datafix:", with what the engine did not get.
func (c *Cluster) sendDirectiveResponse(to sim.StateID, response sim.DirectiveResponse) {
	e, found := c.Find(to)
	if !found {
		logging.Error(
			"datafix: the engine that asked is gone and did not get the response",
			"engine", to, "directive", response.ID, "kind", response.Kind,
			"made_or_removed", response.Engine, "failure", response.Failure, "tag", response.Tag,
		)
		return
	}
	if err := e.HandleDirectiveResponse(response); err != nil {
		logging.Warn(
			"the engine that asked did not take the response, which comes again",
			"engine", to, "directive", response.ID, "err", err,
		)
	}
}

// createEngineLocked makes the engine that the request says, or finds the one that the request
// made before. The state is copied: the request is the one that the engine that asked keeps.
func (c *Cluster) createEngineLocked(
	creator sim.StateID, creatorTick uint64, request sim.DirectiveRequest,
) (sim.StateID, error) {
	key := DirectiveKey{Engine: creator, ID: request.ID}
	if id, done := c.served[key]; done {
		return id, nil
	}
	id, err := c.makeEngineLocked(
		creator, creatorTick, request.ID, request.State.Clone(), request.StartActions,
	)
	if err != nil {
		return 0, err
	}
	c.served[key] = id
	return id, nil
}

func (c *Cluster) makeEngineLocked(
	creator sim.StateID, creatorTick, directiveID uint64, st sim.State,
	start []sim.BaseAction,
) (sim.StateID, error) {
	if c.Env.EngineRegistry == nil {
		return 0, errors.New("the cluster has no registry of engines")
	}
	id := c.nextStateIDLocked(st.Type())
	e, err := c.Env.EngineRegistry.Make(c.Env, id, st, start, creator, creatorTick, directiveID)
	if err != nil {
		return 0, err
	}
	c.addEngineLocked(e)
	return id, nil
}

// ConfigStore is the configuration of the world outside the game, or nil.
func (c *Cluster) ConfigStore() *config.Store { return c.store }

// SyncConfig sends the configuration engine what the store holds, when it is a document that was
// not sent: the command makes the difference from what the engine has, which the cluster knows,
// being the only one that sends it. A store that took no document leaves the engine as it is. When
// the engine cannot take the command now, the next call sends it again. Run calls it; the code that
// steps the engines itself calls it between two steps.
func (c *Cluster) SyncConfig() {
	if c.store == nil {
		return
	}
	cur := c.store.Current()
	if cur.Generation == 0 || cur.Generation == c.syncedConfigGeneration {
		return
	}
	cmd, changed := sim.NewUpdateConfigCommand(c.syncedConfig, cur.Snapshot)
	if changed && !c.config.Send(cmd) {
		return
	}
	c.syncedConfig, c.syncedConfigGeneration = cur.Snapshot, cur.Generation
}

// ConfigEngine returns the configuration engine of the cluster: the one to send an
// UpdateConfigCommand to.
func (c *Cluster) ConfigEngine() *Engine[*sim.ConfigState] { return c.config }

// Engines returns the engines, in the order they were added.
func (c *Cluster) Engines() []EngineHandle {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return slices.Clone(c.engines)
}

// Find returns the engine with the ID, whatever its state is.
func (c *Cluster) Find(id sim.StateID) (EngineHandle, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, e := range c.engines {
		if e.StateID() == id {
			return e, true
		}
	}
	return nil, false
}

// Get returns the engine with the ID as an engine of a state of type S, or false when there is none
// or its state is of another type.
func (c *Cluster) Get[S sim.State](id sim.StateID) (*Engine[S], bool) {
	e, ok := c.Find(id)
	if !ok {
		return nil, false
	}
	typed, ok := e.(*Engine[S])
	return typed, ok
}

// All returns the engines whose state is of type S, in the order they were added.
func (c *Cluster) All[S sim.State]() []*Engine[S] {
	var out []*Engine[S]
	for _, e := range c.Engines() {
		if typed, ok := e.(*Engine[S]); ok {
			out = append(out, typed)
		}
	}
	return out
}

// Run runs every engine of the cluster on a goroutine of its own, ticking every period, the ones
// that are added while it runs included, and returns when the context ends and all of them have
// stopped. An engine is not waited for by the others. When the tick of an engine fails, the whole
// cluster stops as if the context had ended, and the error is logged here, once: Run is where it
// ends. An engine that an action asked to remove ends between two of its ticks on its own
// goroutine, and the cluster drops it.
func (c *Cluster) Run(ctx context.Context, period time.Duration) {
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
			if e.IsClosed() { // it was removed before Run took it
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
				if e.IsClosed() {
					c.Forget(e.StateID())
				}
			})
		}
	}
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()
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
			case <-c.store.Changed():
			case <-t.C: // a command that the engine could not take is sent again
			}
			c.SyncConfig()
		}
	})
	<-ctx.Done()
	c.mu.Lock()
	c.running = false
	c.mu.Unlock()
	c.Env.Router.Stop()
	wg.Wait()
	if failure != nil {
		logging.Error("the cluster stopped", "err", failure)
	}
}
