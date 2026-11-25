package replay

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
)

func TestAMemoryLogOpenedFromATickGivesThatTickAndTheOnesAfterIt(t *testing.T) {
	engine := sim.StateID(1)
	logs := MemoryLogs{engine: {
		{Tick: 1, Body: eventlog.ConfigRead{Version: 1}},
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
