package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
)

// stuckToy is a toy state whose NextTick does not move its tick: a state with a mistake in it.
type stuckToy struct{ toyState }

func (*stuckToy) NextTick() {}

func (s *stuckToy) Clone() sim.State { c := *s; return &c }

func TestAnEngineStartsAtTheTickOfItsState(t *testing.T) {
	e := New(&toyState{id: 1, tick: 40}, newToyEnv(t), 1)
	if e.Tick() != 40 {
		t.Fatalf("the engine is at tick %d, want 40", e.Tick())
	}
	if err := e.Step(); err != nil || e.Tick() != 41 || e.State().Tick() != 41 {
		t.Fatalf("after a step: %v, at %d and %d", err, e.Tick(), e.State().Tick())
	}
}

func TestAStateThatDoesNotMoveItsTickStopsTheClusterAndNothingElseRuns(t *testing.T) {
	c := newCluster(t, newToyEnv(t))
	stuck := New(&stuckToy{toyState{id: 1}}, c.Env, 1)
	after := New(&toyState{id: 2}, c.Env, 1)
	c.Add(stuck)
	c.Add(after)
	err := c.Step()
	if !errors.Is(err, ErrTickMismatch) {
		t.Fatalf("Step = %v, want an error that wraps ErrTickMismatch", err)
	}
	if after.Tick() != 0 {
		t.Fatal("an engine after the one that failed ran its tick")
	}
	for _, r := range c.Env.EventLog.(*eventlog.MemLog).Records {
		if r.Engine == 1 || r.Engine == 2 {
			t.Fatalf("an engine that did not run recorded something of the tick: %+v", r)
		}
	}
	m := sim.NewActionMessage[*toyState](2, onfail.Drop{})
	if reason, ok := c.Env.Router.Deliver(m); ok || reason != sim.DeliveryFailureStopping {
		t.Fatalf("Deliver after the failure = %v, %v: want the cluster stopping", reason, ok)
	}
}

func TestRunStopsEveryEngineWhenOneFails(t *testing.T) {
	c := newCluster(t, newToyEnv(t))
	c.Add(New(&stuckToy{toyState{id: 1}}, c.Env, 1))
	c.Add(New(&toyState{id: 2}, c.Env, 1))
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
