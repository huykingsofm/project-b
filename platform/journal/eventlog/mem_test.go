package eventlog_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
)

func TestAMemoryLogOpenedFromATickGivesThatTickAndTheOnesAfterIt(t *testing.T) {
	engine := sim.StateID(1)
	logs := eventlog.MemoryLogs{engine: {
		{Tick: 1, Body: eventlog.Hash{Value: 0}},
		{Tick: 3, Body: eventlog.Hash{Value: 1}},
		{Tick: 3, Body: eventlog.Hash{Value: 2}},
		{Tick: 5, Body: eventlog.Hash{Value: 3}},
	}}
	for from, want := range map[uint64]int{0: 4, 1: 4, 2: 3, 3: 3, 4: 1, 5: 1, 6: 0} {
		src, err := logs.Open(engine, from)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for src.Next() {
			if src.Record().Tick < from {
				t.Fatalf("from %d: a record of the tick %d", from, src.Record().Tick)
			}
			n++
		}
		if n != want {
			t.Fatalf("from %d: %d records, want %d", from, n, want)
		}
	}
}

func TestAMemLogIsReadAsTheLogsOfItsEngines(t *testing.T) {
	var mem eventlog.MemLog
	a, b := sim.StateID(1), sim.StateID(2)
	mem.Append(b, 1, eventlog.Hash{Value: 1})
	mem.Append(a, 1, eventlog.Hash{Value: 2})
	mem.Append(a, 4, eventlog.Hash{Value: 3})
	if got := mem.Engines(); len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("engines %v, want [1 2]", got)
	}
	if last, err := mem.Last(a); err != nil || last != 4 {
		t.Fatalf("last tick of 1: %d (%v), want 4", last, err)
	}
	src, err := mem.Open(a, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !src.Next() || src.Record().Tick != 4 || src.Next() {
		t.Fatal("a source from the tick 2 does not give the one record of the tick 4")
	}
}
