package engine_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/runtime/cluster"
	"github.com/huykingsofm/project-b/platform/runtime/cluster/stepping"
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/runtime/testkit"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// stuckToy is a toy state whose AdvanceTick does not move its tick: a state with a mistake in it.
type stuckToy struct{ testkit.ToyState }

func (*stuckToy) Type() sim.StateType { return testkit.ToyType + 2 }
func (*stuckToy) AdvanceTick()        {}

func (s *stuckToy) Clone() sim.State { c := *s; return &c }

func decodeStuckToy(r *binary.Reader, version codec.Version) (*stuckToy, error) {
	toy, err := testkit.DecodeToyState(r, version)
	if err != nil {
		return nil, err
	}
	return &stuckToy{*toy}, nil
}

// aClusterWithAStuckToyAndAToy is a cluster that makes the engine of a stuck toy first, and then
// the engine of a toy, and the numbers of the two.
func aClusterWithAStuckToyAndAToy(t *testing.T) (*cluster.Cluster, sim.StateID, sim.StateID) {
	t.Helper()
	codecs, engines, err := testkit.ToyCodecs()
	if err != nil {
		t.Fatal(err)
	}
	if err := engines.Register(codecs, decodeStuckToy); err != nil {
		t.Fatal(err)
	}
	env := testkit.NewToyEnv(t)
	env.Codecs, env.EngineRegistry = codecs, engines
	c := testkit.NewCluster(t, env)
	stuck := testkit.AddEngineOf(t, c, &stuckToy{})
	after := testkit.AddEngineOf(t, c, &testkit.ToyState{})
	return c, stuck.StateID(), after.StateID()
}

func TestAnEngineStartsAtTheTickOfItsState(t *testing.T) {
	e := engine.NewEngine(testkit.NewToyStateAtTick(40), 1, testkit.NewToyEnv(t))
	if e.Tick() != 40 {
		t.Fatalf("the engine is at tick %d, want 40", e.Tick())
	}
	if err := e.Step(); err != nil || e.Tick() != 41 || e.State().Tick() != 41 {
		t.Fatalf("after a step: %v, at %d and %d", err, e.Tick(), e.State().Tick())
	}
}

func TestAStateThatDoesNotMoveItsTickStopsTheClusterAndNothingElseRuns(t *testing.T) {
	c, stuck, afterID := aClusterWithAStuckToyAndAToy(t)
	after, _ := c.FindOfType[*testkit.ToyState](afterID)
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
	m := sim.NewActionMessage[*testkit.ToyState](afterID, onfail.Drop{})
	if reason, ok := c.Env.Router.Deliver(m); ok || reason != sim.DeliveryFailureStopping {
		t.Fatalf("Deliver after the failure = %v, %v: want the cluster stopping", reason, ok)
	}
}

func TestRunStopsEveryEngineWhenOneFails(t *testing.T) {
	c, _, _ := aClusterWithAStuckToyAndAToy(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		if err := c.Run(ctx, time.Millisecond); err == nil {
			t.Error("the cluster stopped with no error, though an engine failed")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("the cluster did not stop when an engine failed")
	}
}
