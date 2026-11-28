package engine_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/runtime/cluster"
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/runtime/testkit"
	"github.com/huykingsofm/project-b/platform/sim"
)

// readsOf is the copy ticks that the engine read of the source, as its event log says, in order.
func readsOf(c *cluster.Cluster, reader, source sim.StateID) []uint64 {
	var out []uint64
	for _, r := range c.Env.EventLog.(*aLogThatSaysWhatIsSafe).Records {
		read, ok := r.Body.(eventlog.StableRead)
		if ok && r.Engine == reader && read.Source == source {
			out = append(out, read.CopyTick)
		}
	}
	return out
}

func TestAnEngineThatWasRemovedIsReadAsItsLastTickBeforeAndAsNothingAfter(t *testing.T) {
	c, a, b, _ := threeToys(t)
	for tick := uint64(1); tick <= 3; tick++ {
		b.ScheduleAt(tick, testkit.ToyAction{N: 1})
	}
	a.ScheduleAt(4, testkit.ReadAction{From: 2})
	stepAll(t, []*engine.Engine[*testkit.ToyState]{b, b, b}, 1) // three ticks of b
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a, a, a, a}, 1)
	if a.State().Total != 3 {
		t.Fatalf("a read %d of b, want 3: the copy of its last tick", a.State().Total)
	}
	b.Retire()
	if b.Tick() != 4 || b.State().Tick() != 3 {
		t.Fatalf("b is at the tick %d and its state at %d, want 4 and 3: it ran no tick",
			b.Tick(), b.State().Tick())
	}
	a.ScheduleAt(5, testkit.ReadAction{From: 2})
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a}, 1)
	if a.State().Total != 3 {
		t.Fatalf("a has %d after b was removed, want 3: nothing was read", a.State().Total)
	}
	if got := readsOf(c, 1, 2); len(got) != 2 || got[0] != 3 || got[1] != 0 {
		t.Fatalf("a read the copies of the ticks %v of b, want 3 and then none (0)", got)
	}
}
