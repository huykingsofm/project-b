package cluster_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/runtime/cluster/stepping"
	"github.com/huykingsofm/project-b/platform/runtime/testkit"
	"github.com/huykingsofm/project-b/platform/sim"
)

// writerThatNotesTheTicks is a checkpoint writer that writes nothing: it notes the engines that
// offered a checkpoint at each tick.
type writerThatNotesTheTicks struct {
	enginesAt map[uint64][]sim.StateID
}

func (w *writerThatNotesTheTicks) Offer(engine sim.StateID, tick uint64, _ func() ([]byte, error)) {
	w.enginesAt[tick] = append(w.enginesAt[tick], engine)
}

func TestTheCheckpointsOfTheEnginesOfAClusterDoNotFallOnTheSameTick(t *testing.T) {
	const ticksRun, periodTicks = 1200, 600 // the period is the default of checkpoint.every_ticks
	for _, toys := range []int{6, 14, 40} {
		writer := &writerThatNotesTheTicks{enginesAt: map[uint64][]sim.StateID{}}
		env := testkit.NewToyEnv(t)
		env.CheckpointWriter = writer
		c := testkit.NewCluster(t, env)
		testkit.AddToys(t, c, toys)
		for range ticksRun {
			if err := stepping.Step(c); err != nil {
				t.Fatal(err)
			}
		}
		taken, peak := map[sim.StateID]int{}, 0
		for _, engines := range writer.enginesAt {
			peak = max(peak, len(engines))
			for _, id := range engines {
				taken[id]++
			}
		}
		engines := toys + 1 // the toys and the coordinator engine
		if len(taken) != engines {
			t.Fatalf("%d engines took a checkpoint, want all %d", len(taken), engines)
		}
		for id, count := range taken {
			if count != ticksRun/periodTicks {
				t.Fatalf("%v took %d checkpoints in %d ticks, want %d",
					id, count, ticksRun, ticksRun/periodTicks)
			}
		}
		if peak > 2 {
			t.Fatalf("%d engines took a checkpoint at one tick, among %d engines, want at most 2",
				peak, engines)
		}
	}
}
