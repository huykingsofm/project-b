package engine

import (
	"errors"
	"testing"

	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
)

func publishedAt(stable *StableStateStore, tick uint64) *toyState {
	st := &toyState{id: 7, tick: tick} // a toy state is made at any tick
	stable.Publish(st)
	return st
}

func readToy(t *testing.T, view *StableView, id sim.StateID) *toyState {
	t.Helper()
	got, err := view.Read(id)
	if err != nil {
		t.Fatalf("Read(%d): %v", id, err)
	}
	return got.(*toyState)
}

func TestWhatAnEngineReadsOfAnotherStaysTheSameForTheTick(t *testing.T) {
	stable := NewStableStateStore()
	eventLog := &eventlog.MemLog{}
	view := stable.View(1, eventLog)

	first := publishedAt(stable, 3)
	view.begin(10)
	if got := readToy(t, view, 7); got.Tick() != first.Tick() {
		t.Fatal("the first read did not return the latest copy")
	}
	publishedAt(stable, 4) // the other engine publishes while this tick runs
	if got := readToy(t, view, 7); got.Tick() != 3 {
		t.Fatal("a later read in the same tick returned another copy")
	}

	view.begin(11)
	if got := readToy(t, view, 7); got.Tick() != 4 {
		t.Fatal("the next tick did not read the newer copy")
	}
}

func TestAStateThatWasNotPublishedIsNotFoundAndTheReadIsKeptForTheTick(t *testing.T) {
	stable := NewStableStateStore()
	view := stable.View(1, &eventlog.MemLog{})
	view.begin(10)
	if _, err := view.Read(8); !errors.Is(err, sim.ErrStateNotFound) {
		t.Fatalf("a state that nobody published: %v", err)
	}
	stable.Publish(&toyState{id: 8})
	if _, err := view.Read(8); !errors.Is(err, sim.ErrStateNotFound) {
		t.Fatal("a state published later in the tick was found by an engine that did not find it")
	}
	view.begin(11)
	if _, err := view.Read(8); err != nil {
		t.Fatalf("the next tick did not find it: %v", err)
	}
	stable.Remove(8)
	view.begin(12)
	if _, err := view.Read(8); !errors.Is(err, sim.ErrStateNotFound) {
		t.Fatalf("a state that was removed: %v", err)
	}
}

func TestTheFirstReadOfEachStateInATickIsWrittenToTheEventLog(t *testing.T) {
	stable := NewStableStateStore()
	eventLog := &eventlog.MemLog{}
	view := stable.View(1, eventLog)
	publishedAt(stable, 3)

	view.begin(10)
	view.Read(7)
	view.Read(7) // the same state again: not written again
	view.Read(8) // nothing published: written with copy tick 0
	view.begin(11)
	view.Read(7)

	var got []eventlog.Record
	for _, r := range eventLog.Records {
		if r.Kind() == eventlog.KindStableRead {
			got = append(got, r)
		}
	}
	read := func(tick uint64, source sim.StateID, copyTick uint64) eventlog.Record {
		return eventlog.Record{
			Engine: 1, Tick: tick, Body: eventlog.StableRead{Source: source, CopyTick: copyTick},
		}
	}
	want := []eventlog.Record{read(10, 7, 3), read(10, 8, 0), read(11, 7, 3)}
	if len(got) != len(want) {
		t.Fatalf("records %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("record %d is %+v, want %+v", i, got[i], want[i])
		}
	}
}

// readsOf is the copy ticks that the engine read of the source, as its event log says, in order.
func readsOf(c *Cluster, reader, source sim.StateID) []uint64 {
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
		b.ScheduleAt(tick, toyAction{N: 1})
	}
	a.ScheduleAt(4, readAction{From: 2})
	stepAll(t, []*Engine[*toyState]{b, b, b}, 1) // three ticks of b
	stepAll(t, []*Engine[*toyState]{a, a, a, a}, 1)
	if a.State().Total != 3 {
		t.Fatalf("a read %d of b, want 3: the copy of its last tick", a.State().Total)
	}
	b.Remove()
	if b.Tick() != 4 || b.State().Tick() != 3 {
		t.Fatalf("b is at the tick %d and its state at %d, want 4 and 3: it ran no tick",
			b.Tick(), b.State().Tick())
	}
	a.ScheduleAt(5, readAction{From: 2})
	stepAll(t, []*Engine[*toyState]{a}, 1)
	if a.State().Total != 3 {
		t.Fatalf("a has %d after b was removed, want 3: nothing was read", a.State().Total)
	}
	if got := readsOf(c, 1, 2); len(got) != 2 || got[0] != 3 || got[1] != 0 {
		t.Fatalf("a read the copies of the ticks %v of b, want 3 and then none (0)", got)
	}
}
