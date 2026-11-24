package engine

import (
	"errors"
	"testing"

	"github.com/huykingsofm/project-b/platform/replay"
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
	replayLog := &replay.MemLog{}
	view := stable.View(1, replayLog)

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
	view := stable.View(1, &replay.MemLog{})
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

func TestTheFirstReadOfEachStateInATickIsWrittenToTheReplayLog(t *testing.T) {
	stable := NewStableStateStore()
	replayLog := &replay.MemLog{}
	view := stable.View(1, replayLog)
	publishedAt(stable, 3)

	view.begin(10)
	view.Read(7)
	view.Read(7) // the same state again: not written again
	view.Read(8) // nothing published: written with copy tick 0
	view.begin(11)
	view.Read(7)

	var got []replay.Record
	for _, r := range replayLog.Records {
		if r.Kind() == replay.KindStableRead {
			got = append(got, r)
		}
	}
	read := func(tick uint64, source sim.StateID, copyTick uint64) replay.Record {
		return replay.Record{
			Engine: 1, Tick: tick, Body: replay.StableRead{Source: source, CopyTick: copyTick},
		}
	}
	want := []replay.Record{read(10, 7, 3), read(10, 8, 0), read(11, 7, 3)}
	if len(got) != len(want) {
		t.Fatalf("records %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("record %d is %+v, want %+v", i, got[i], want[i])
		}
	}
}
