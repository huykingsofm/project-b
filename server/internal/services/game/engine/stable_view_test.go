package engine

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
)

func publishedAt(stable *StableStateRegistry, tick uint64) *zonelayerstate.State {
	st := zonelayerstate.NewState(7, 1)
	st.SetTick(tick)
	stable.PublishZoneLayer(st)
	return st
}

func TestWhatAnEngineReadsOfAnotherStaysTheSameForTheTick(t *testing.T) {
	stable := NewStableStateRegistry(1)
	replayLog := &replay.MemLog{}
	view := stable.View(action.ZoneAddress(1), replayLog)

	first := publishedAt(stable, 3)
	view.begin(10)
	if got := view.ZoneLayer(7); got != first {
		t.Fatal("the first read did not return the latest copy")
	}
	newer := publishedAt(stable, 4) // the other engine publishes while this tick runs
	if got := view.ZoneLayer(7); got != first {
		t.Fatal("a later read in the same tick returned another copy")
	}

	view.begin(11)
	if got := view.ZoneLayer(7); got != newer {
		t.Fatal("the next tick did not read the newer copy")
	}
}

func TestTheFirstReadOfEachScopeInATickIsWrittenToTheReplayLog(t *testing.T) {
	stable := NewStableStateRegistry(1)
	replayLog := &replay.MemLog{}
	view := stable.View(action.ZoneAddress(1), replayLog)
	publishedAt(stable, 3)

	view.begin(10)
	view.ZoneLayer(7)
	view.ZoneLayer(7) // the same scope again: not written again
	view.ZoneLayer(8) // nothing published: written with copy tick 0
	view.begin(11)
	view.ZoneLayer(7)

	var got []replay.Record
	for _, r := range replayLog.Records {
		if r.Kind() == replay.KindStableRead {
			got = append(got, r)
		}
	}
	read := func(tick uint64, source action.Address, copyTick uint64) replay.Record {
		return replay.Record{
			Engine: action.ZoneAddress(1), Tick: tick,
			Body: replay.StableRead{Source: source, CopyTick: copyTick},
		}
	}
	want := []replay.Record{
		read(10, action.ZoneLayerAddress(7), 3),
		read(10, action.ZoneLayerAddress(8), 0),
		read(11, action.ZoneLayerAddress(7), 3),
	}
	if len(got) != len(want) {
		t.Fatalf("%d records, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("record %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
