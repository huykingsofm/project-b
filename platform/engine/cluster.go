package engine

import (
	"context"
	"errors"
	"fmt"
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
	// Starter is where the coordinator hands the engines it makes and the ones it ends: NewCluster
	// sets it to the cluster. A replay has none, and its coordinator makes no engine.
	Starter EngineStarter
}

// EngineHandle is what the cluster needs of an engine, whatever the type of its state is: to run
// it on its own, to end it and to say which engine it is. What the code that holds an engine from
// the cluster (Cluster.Engines, Cluster.Find, Cluster.AddEngineByCheckpoint) needs besides, it asks
// of the handle itself: it defines its own interface and converts the handle to it.
type EngineHandle interface {
	StateID() sim.StateID
	// Run runs the engine on its own until the context ends or its tick fails.
	Run(ctx context.Context, ticks <-chan time.Time) error
	// Retire ends the engine for good and writes the record of it to its log: see Engine.Retire.
	Retire()
	// RequestRetirement asks the engine to retire between two of its ticks, at the tick of the
	// coordinator that asked: Run does it, or the code that steps the engines itself
	// (Engine.RetireIfRequested); it does not wait, and may be called from any goroutine.
	RequestRetirement(by sim.StateDependency)
	// IsClosed says that the engine has been closed: it retired or was closed.
	IsClosed() bool
}

// Checkpoints are the checkpoints that the two engines every cluster has are made from; a nil one
// makes its engine new.
type Checkpoints struct {
	Config      []byte
	Coordinator []byte
}

// Cluster is the engines of a game, wired. It knows only that they are engines: what each does is
// the business of its state. The first of them are the configuration engine and the coordinator,
// which every cluster has; the coordinator makes and ends the others, which the engines ask it for
// with a message and the code that runs the cluster with a command (CreateEngine).
type Cluster struct {
	Env         *Environment
	config      *Engine[*sim.ConfigState]
	coordinator *Engine[*sim.CoordinatorState]

	// mu protects the list of engines, enginesToRun and running: the coordinator makes an engine on
	// its own goroutine, and the cluster drops an engine from the list when it has ended.
	mu      sync.RWMutex
	engines []EngineHandle
	// enginesToRun are the engines that Run has not taken yet, every engine that is added (before
	// Run or while it runs), and wakeRun wakes Run when one is added.
	enginesToRun []EngineHandle
	wakeRun      chan struct{}
	// running says that Run runs the engines on their own goroutines, and runEnded is closed when
	// it stops.
	running  bool
	runEnded <-chan struct{}
	// stepMu is held while CreateEngine takes a tick of the coordinator itself, and while Run
	// starts: a tick is never taken by both
	stepMu sync.Mutex

	// waiters are the callers of CreateEngine that wait for the ID of their engine, by the ticket
	// that their command holds
	waitersMu  sync.Mutex
	waiters    map[uint64]chan madeEngine
	lastTicket uint64

	// store is the configuration of the world outside the game, which the cluster brings into the
	// configuration engine; nil means none. syncedConfig is the configuration that the engine has,
	// or is sent, from it, and syncedConfigGeneration the generation of the document that made it.
	store                  *config.Store
	syncedConfig           config.ReadOnlySnapshot
	syncedConfigGeneration uint64
}

var _ EngineStarter = (*Cluster)(nil)

// NewCluster makes a cluster that runs, in the environment: it has its configuration engine and its
// coordinator and no other, made from the checkpoints given or, where one is nil, new. The cluster
// becomes where the coordinator hands the engines it makes (Environment.Starter). The environment's
// codecs and engine registry must know the two (see NewCodecs and NewEngineRegistry). The store is
// the configuration of the world outside the game, which the cluster sends to the configuration
// engine when it changes; nil means none.
func NewCluster(env *Environment, store *config.Store, from Checkpoints) (*Cluster, error) {
	c, err := buildCluster(env, store, from)
	if err != nil {
		return nil, err
	}
	env.Starter = c
	return c, nil
}

// NewReplayCluster makes the cluster of a replay: as NewCluster does, but its coordinator makes
// and ends no engine, since the logs say which engines were made and removed and when.
func NewReplayCluster(env *Environment, from Checkpoints) (*Cluster, error) {
	env.Starter = nil
	return buildCluster(env, nil, from)
}

func buildCluster(env *Environment, store *config.Store, from Checkpoints) (*Cluster, error) {
	configEngine, err := rootEngine(env, from.Config, sim.NewConfigState(), sim.ConfigStateID)
	if err != nil {
		return nil, fmt.Errorf("the configuration engine: %w", err)
	}
	coordinator, err := rootEngine(
		env, from.Coordinator, sim.NewCoordinatorState(), sim.CoordinatorStateID,
	)
	if err != nil {
		return nil, fmt.Errorf("the coordinator: %w", err)
	}
	c := &Cluster{
		Env: env, config: configEngine, coordinator: coordinator, wakeRun: make(chan struct{}, 1),
		waiters: map[uint64]chan madeEngine{}, store: store,
		syncedConfig: configEngine.State().Snapshot(),
	}
	c.addEngineLocked(configEngine)
	c.addEngineLocked(coordinator)
	return c, nil
}

// rootEngine makes one of the engines that every cluster has: from the checkpoint, or new from the
// state when there is none.
func rootEngine[S sim.State](
	env *Environment, checkpoint []byte, fresh S, id sim.StateID,
) (*Engine[S], error) {
	if checkpoint == nil {
		return NewEngine(fresh, id, env), nil
	}
	decoded, err := DecodeEngineCheckpoint[S](checkpoint, env.Codecs)
	if err != nil {
		return nil, err
	}
	return decoded.Restore(env)
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

// Forget drops an engine from the list of the cluster. It is for an engine that has retired: it is
// not retired by this, and the router still knows it. Run calls it for the engines it runs, and the
// code that steps the engines itself calls it for the ones that retired between two steps.
func (c *Cluster) Forget(id sim.StateID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.engines = slices.DeleteFunc(c.engines, func(e EngineHandle) bool { return e.StateID() == id })
}

// CreateEngine has the coordinator make an engine of the state, with the actions it starts with,
// and returns its ID: it is for the code that runs the cluster, as the messages of the engines are
// for the engines. The state has no ID: the coordinator gives the engine one. The engine is new, at
// the tick of the state, and registered with the router; its log begins with a record that holds it
// as it is, which depends on the tick of the coordinator. When the cluster runs on its goroutines
// the call waits for the next tick of the coordinator; otherwise it takes that tick itself, so the
// engine is there when the call returns, and the first step of the cluster runs it.
func (c *Cluster) CreateEngine(
	st sim.State, start ...sim.BaseAction,
) (sim.StateID, error) {
	c.waitersMu.Lock()
	c.lastTicket++
	ticket, made := c.lastTicket, make(chan madeEngine, 1)
	c.waiters[ticket] = made
	c.waitersMu.Unlock()
	cmd, err := sim.NewCreateEngineCommand(st, start, ticket)
	if err != nil {
		c.dropWaiter(ticket)
		return 0, err
	}
	if !c.coordinator.Send(cmd) {
		c.dropWaiter(ticket)
		return 0, ErrCommandQueueFull
	}
	if err := c.stepCoordinatorUnlessRunning(); err != nil {
		c.dropWaiter(ticket)
		return 0, err
	}
	c.mu.RLock()
	ended := c.runEnded
	c.mu.RUnlock()
	select {
	case m := <-made:
		return m.id, m.err
	case <-ended:
		c.dropWaiter(ticket)
		return 0, errors.New("the cluster stopped before it made the engine")
	}
}

// stepCoordinatorUnlessRunning takes a tick of the coordinator when the cluster does not run its
// engines on goroutines: there is no one else to take it.
func (c *Cluster) stepCoordinatorUnlessRunning() error {
	c.stepMu.Lock()
	defer c.stepMu.Unlock()
	c.mu.RLock()
	running := c.running
	c.mu.RUnlock()
	if running {
		return nil
	}
	return c.coordinator.Step()
}

// dropWaiter forgets a caller of CreateEngine that does not wait any more.
func (c *Cluster) dropWaiter(ticket uint64) {
	c.waitersMu.Lock()
	defer c.waitersMu.Unlock()
	delete(c.waiters, ticket)
}

// EngineMade takes an engine that the coordinator made and runs it from now on, and tells the
// caller of CreateEngine that waits for it, if there is one: the engine that is made now is there
// for a message of the tick to find it.
func (c *Cluster) EngineMade(ticket uint64, e EngineHandle, err error) {
	var id sim.StateID
	if err == nil {
		id = e.StateID()
		c.mu.Lock()
		c.addEngineLocked(e)
		c.mu.Unlock()
	}
	c.waitersMu.Lock()
	made, waited := c.waiters[ticket]
	delete(c.waiters, ticket)
	c.waitersMu.Unlock()
	switch {
	case waited:
		made <- madeEngine{id: id, err: err}
	case err != nil:
		logging.Error("the coordinator could not make an engine", "err", err)
	}
}

// RetireEngine asks the engine with the ID to retire between two of its ticks, at the tick of the
// coordinator that asked. It does nothing for an ID that no engine has.
func (c *Cluster) RetireEngine(id sim.StateID, by sim.StateDependency) {
	if e, found := c.Find(id); found {
		e.RequestRetirement(by)
	}
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

// Coordinator returns the coordinator engine of the cluster: the one to send a CreateEngineCommand
// or a RetireEngineCommand to.
func (c *Cluster) Coordinator() *Engine[*sim.CoordinatorState] { return c.coordinator }

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
	c.stepMu.Lock()
	c.mu.Lock()
	c.running, c.runEnded = true, ctx.Done()
	c.mu.Unlock()
	c.stepMu.Unlock()
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

// madeEngine is what a caller of CreateEngine waits for.
type madeEngine struct {
	id  sim.StateID
	err error
}
