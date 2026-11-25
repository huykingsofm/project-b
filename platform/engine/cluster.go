package engine

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/logging"
)

// Env is what every engine of a cluster shares: the router that carries their messages, the replay
// log they write, the store of the states they publish (the configuration is read from it, from
// the state of the configuration engine), the emitter where they give what changed, the codecs that
// read back what they wrote, and the writer of their checkpoints.
type Env struct {
	Router    MessageRouter
	EventLog  eventlog.Log
	Stable    StableStore
	Codecs    *sim.Codecs
	QueueSize int
	Emitter   Emitter
	// Acks is where the engines say what they hold safe and read what their receivers hold. An
	// engine with none holds no message for an ack: a replay has none.
	Acks *AckBoard
	// Checkpoints writes the checkpoints that the engines take at the end of a tick. With none the
	// engines take none: a replay has none.
	Checkpoints CheckpointWriter
}

// Remove ends the engine with the state ID for good, and forgets it: see Engine.Remove. It reports
// whether the cluster had one.
func (c *Cluster) Remove(id sim.StateID) bool {
	for i, e := range c.engines {
		if e.StateID() == id {
			e.Remove()
			c.engines = slices.Delete(c.engines, i, i+1)
			return true
		}
	}
	return false
}

// Stepper is what a cluster needs of an engine, whatever its state is: to run a tick, to run on
// its own, and to say which engine it is.
type Stepper interface {
	Step() error
	Run(ctx context.Context, ticks <-chan time.Time) error
	// Remove ends the engine for good: see Engine.Remove.
	Remove()
	StateID() sim.StateID
	Type() sim.StateType
	// TypeName is the name of the type of the state of the engine, for a message to a person.
	TypeName() string
	// SendBaseCommand gives the engine a command that was read without its type, as a replay does.
	SendBaseCommand(c sim.BaseCommand) error
	// Tick is the tick the engine is at: the last it ran, or the one it was made at.
	Tick() uint64
	// Hash is the hash of what the engine goes on from (see Engine.Hash).
	Hash(h sim.Hasher) uint64
	// SentMessages are the messages the engine sent and holds until their receivers say they hold
	// them safe.
	SentMessages() []sim.Message
	// PublishState publishes a copy of the state as it is, for the other engines to read: an
	// engine that was made from a checkpoint has not run the tick that publishes it.
	PublishState()
}

var _ Stepper = (*Engine[sim.State])(nil)

// Cluster is the engines of a game, built and wired. It knows only that they are engines: what each
// does is the business of its state. The first of them is the configuration engine, which every
// cluster has.
type Cluster struct {
	Env     *Env
	config  *Engine[*sim.ConfigState]
	engines []Stepper
}

// NewCluster makes a cluster that has its configuration engine and no other, in the environment:
// from the checkpoint that the source has for it, or new with every variable at its default. It
// returns an error when that checkpoint cannot make the engine. The environment's codecs must know
// the configuration engine (see NewCodecs).
func NewCluster(env *Env, seed uint64, checkpoints CheckpointSource) (*Cluster, error) {
	ce, err := NewOrRestore(sim.NewConfigState(), env, seed, checkpoints)
	if err != nil {
		return nil, fmt.Errorf("the configuration engine: %w", err)
	}
	return &Cluster{Env: env, config: ce, engines: []Stepper{ce}}, nil
}

// ConfigEngine returns the configuration engine of the cluster: the one to send an
// UpdateConfigCommand to.
func (c *Cluster) ConfigEngine() *Engine[*sim.ConfigState] { return c.config }

// Reader reads the latest copy that each engine published, with no lock and without the log: for
// whoever is outside the engines and asks (a gateway, a config service). An engine reads through
// its own view, which the event log records. It finds nothing in the environment of a replay.
func (c *Cluster) Reader() sim.StableReader {
	if r, ok := c.Env.Stable.(sim.StableReader); ok {
		return r
	}
	return noCopies{}
}

// Config is the configuration as the configuration engine last published it, or the defaults when
// it published none.
func (c *Cluster) Config() config.ReadOnlySnapshot {
	if st, err := c.Reader().Read(sim.ConfigStateID); err == nil {
		if cs, ok := st.(*sim.ConfigState); ok {
			return cs.Snapshot()
		}
	}
	return config.Defaults()
}

// noCopies is the reader of an environment that keeps no copy to read.
type noCopies struct{}

func (noCopies) Read(sim.StateID) (sim.State, error) { return nil, sim.ErrStateNotFound }

// Add puts an engine in the cluster, after the engines that are already in it.
func (c *Cluster) Add(e Stepper) { c.engines = append(c.engines, e) }

// Engines returns the engines, in the order they were added.
func (c *Cluster) Engines() []Stepper { return slices.Clone(c.engines) }

// Find returns the engine with the ID, whatever its state is.
func (c *Cluster) Find(id sim.StateID) (Stepper, bool) {
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
	for _, e := range c.engines {
		if typed, ok := e.(*Engine[S]); ok {
			out = append(out, typed)
		}
	}
	return out
}

// Step runs one tick of every engine, one after the other, in the order they were added. A message
// that an engine sends is in the In channel of the receiver at once, so an engine that comes later
// reads in this step what an earlier one sent in it, and an earlier one reads in the next step what
// a later one sent. It is the way to run the engines with a fixed order, as a test or an example
// needs; Run lets each engine go at its own pace. When the tick of an engine fails the cluster
// stops: the router refuses every message from then on, the engines after it do not run, and the
// error is returned.
func (c *Cluster) Step() error {
	for _, e := range c.engines {
		if err := e.Step(); err != nil {
			c.Env.Router.Stop()
			return fmt.Errorf("the cluster stopped: %w", err)
		}
	}
	return nil
}

// Run starts every engine on a goroutine of its own, ticking every period, and returns when the
// context ends and all of them have stopped. An engine is not waited for by the others. When the
// tick of an engine fails, the whole cluster stops as if the context had ended, and the error is
// logged here, once: Run is where it ends.
func (c *Cluster) Run(ctx context.Context, period time.Duration) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		wg       sync.WaitGroup
		failOnce sync.Once
		failure  error
	)
	for _, e := range c.engines {
		wg.Go(func() {
			t := time.NewTicker(period)
			defer t.Stop()
			if err := e.Run(ctx, t.C); err != nil {
				failOnce.Do(func() {
					failure = err
					cancel()
				})
			}
		})
	}
	<-ctx.Done()
	c.Env.Router.Stop()
	wg.Wait()
	if failure != nil {
		logging.Error("the cluster stopped", "err", failure)
	}
}
