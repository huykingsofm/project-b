package runner

import (
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/engine/enginetest"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
)

// The copies that a replay keeps are those that a read still to come will ask for: when the run is
// over every read has been made, and none is kept.
func TestACopyIsLetGoOfAsSoonAsItsLastReadIsMade(t *testing.T) {
	c, _, mem, cfg := enginetest.NewWorld(t, 7, 10)
	enginetest.RunScenario(t, c, cfg, enginetest.Sender{T: t})
	logs := MemoryLogs{}
	for _, r := range mem.Records {
		logs[r.Engine] = append(logs[r.Engine], r)
	}
	reads := 0
	for _, records := range logs {
		for _, r := range records {
			if read, ok := r.Body.(replay.StableRead); ok && read.CopyTick > 0 {
				reads++
			}
		}
	}
	if reads == 0 {
		t.Fatal("the scenario reads no copy of another engine")
	}

	run, err := newReplayRun(enginetest.WorldBuilder(t, 7, 10), logs, scenarioConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer run.close()
	if err := run.play(); err != nil {
		t.Fatal(err)
	}
	if held := run.stable.held(); held != 0 {
		t.Fatalf("%d copies are still kept when the run is over, after %d reads", held, reads)
	}
	if len(run.stable.uses) != 0 || len(run.stable.reads) != 0 {
		t.Fatalf(
			"%d copies still wait for a read and %d reads are still to come",
			len(run.stable.uses), len(run.stable.reads),
		)
	}
	if len(run.div.list()) != 0 {
		t.Fatalf("the replay differs from the log: %v", run.div.list())
	}
}

var scenarioConfig = Options{
	ConfigVersions: map[uint64]map[string]string{
		1: {"zonelayer.normal_attack.cooldown_ticks": "5"},
	},
}
