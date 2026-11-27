package engine_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/engine/stepping"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// stuckToy is a toy state whose NextTick does not move its tick: a state with a mistake in it.
type stuckToy struct{ toyState }

func (*stuckToy) Type() sim.StateType { return toyType + 2 }
func (*stuckToy) NextTick()           {}

func (s *stuckToy) Clone() sim.State { c := *s; return &c }

func decodeStuckToy(r *binary.Reader, version codec.Version) (*stuckToy, error) {
	toy, err := decodeToyState(r, version)
	if err != nil {
		return nil, err
	}
	return &stuckToy{*toy}, nil
}

// aClusterWithAStuckToyAndAToy is a cluster that makes the engine of a stuck toy first, and then
// the engine of a toy, and the numbers of the two.
func aClusterWithAStuckToyAndAToy(t *testing.T) (*engine.Cluster, sim.StateID, sim.StateID) {
	t.Helper()
	codecs, engines, err := toyCodecs()
	if err != nil {
		t.Fatal(err)
	}
	if err := engines.Register(codecs, decodeStuckToy); err != nil {
		t.Fatal(err)
	}
	env := newToyEnv(t)
	env.Codecs, env.EngineRegistry = codecs, engines
	c := newCluster(t, env)
	stuck, err := c.CreateEngine(&stuckToy{})
	if err != nil {
		t.Fatal(err)
	}
	after, err := c.CreateEngine(&toyState{})
	if err != nil {
		t.Fatal(err)
	}
	return c, stuck, after
}

func TestAnEngineStartsAtTheTickOfItsState(t *testing.T) {
	e := engine.NewEngine(&toyState{tick: 40}, 1, newToyEnv(t))
	if e.Tick() != 40 {
		t.Fatalf("the engine is at tick %d, want 40", e.Tick())
	}
	if err := e.Step(); err != nil || e.Tick() != 41 || e.State().Tick() != 41 {
		t.Fatalf("after a step: %v, at %d and %d", err, e.Tick(), e.State().Tick())
	}
}

func TestAStateThatDoesNotMoveItsTickStopsTheClusterAndNothingElseRuns(t *testing.T) {
	c, stuck, afterID := aClusterWithAStuckToyAndAToy(t)
	after, _ := c.Get[*toyState](afterID)
	err := stepping.Step(c)
	if !errors.Is(err, engine.ErrTickMismatch) {
		t.Fatalf("Step = %v, want an error that wraps ErrTickMismatch", err)
	}
	if after.Tick() != 0 {
		t.Fatal("an engine after the one that failed ran its tick")
	}
	for _, r := range c.Env.EventLog.(*eventlog.MemLog).Records {
		if r.Engine == stuck || r.Engine == afterID {
			t.Fatalf("an engine that did not run recorded something of the tick: %+v", r)
		}
	}
	m := sim.NewActionMessage[*toyState](afterID, onfail.Drop{})
	if reason, ok := c.Env.Router.Deliver(m); ok || reason != sim.DeliveryFailureStopping {
		t.Fatalf("Deliver after the failure = %v, %v: want the cluster stopping", reason, ok)
	}
}

func TestRunStopsEveryEngineWhenOneFails(t *testing.T) {
	c, _, _ := aClusterWithAStuckToyAndAToy(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { c.Run(ctx, time.Millisecond); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("the cluster did not stop when an engine failed")
	}
}
