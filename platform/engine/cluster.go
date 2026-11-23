package engine

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
)

// Env is what every engine of a cluster shares: the router that carries their messages, the
// configuration they read, the replay log they write, the store of the states they publish, the
// emitter where they give what changed, and the codecs that read back what they wrote.
type Env struct {
	Router    MessageRouter
	Cfg       ConfigSource
	ReplayLog replay.Log
	Stable    StableStore
	Codecs    *sim.Codecs
	QueueSize int
	Emitter   Emitter
}

// Stepper is what a cluster needs of an engine, whatever its state is: to run a tick, to run on
// its own, and to say which engine it is.
type Stepper interface {
	Step()
	Run(ctx context.Context, ticks <-chan time.Time)
	StateID() sim.StateID
	Type() sim.StateType
	// TypeName is the name of the type of the state of the engine, for a message to a person.
	TypeName() string
	// SendBaseCommand gives the engine a command that was read without its type, as a replay does.
	SendBaseCommand(c sim.BaseCommand) error
}

var _ Stepper = (*Engine[sim.State])(nil)

// Cluster is the engines of a game, built and wired. It knows only that they are engines: what each
// does is the business of its state.
type Cluster struct {
	Env     *Env
	engines []Stepper
}

// NewCluster makes an empty cluster in the environment.
func NewCluster(env *Env) *Cluster { return &Cluster{Env: env} }

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
// needs; Run lets each engine go at its own pace.
func (c *Cluster) Step() {
	for _, e := range c.engines {
		e.Step()
	}
}

// Run starts every engine on a goroutine of its own, ticking every period, and returns when the
// context ends and all of them have stopped. An engine is not waited for by the others.
func (c *Cluster) Run(ctx context.Context, period time.Duration) {
	var wg sync.WaitGroup
	for _, e := range c.engines {
		wg.Go(func() {
			t := time.NewTicker(period)
			defer t.Stop()
			e.Run(ctx, t.C)
		})
	}
	<-ctx.Done()
	c.Env.Router.Stop()
	wg.Wait()
}
