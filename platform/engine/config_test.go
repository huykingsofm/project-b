package engine

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
)

// configReads returns, for each tick of the engine, the tick of the copy of the configuration
// engine that it read, as its log says.
func configReads(mem *eventlog.MemLog, engine sim.StateID) map[uint64]uint64 {
	out := map[uint64]uint64{}
	for _, r := range mem.Records {
		if read, ok := r.Body.(eventlog.StableRead); ok && r.Engine == engine &&
			read.Source == sim.ConfigStateID {
			out[r.Tick] = read.CopyTick
		}
	}
	return out
}

func TestAnEngineReadsTheConfigurationFromTheCopyOfTheConfigurationEngineAtEveryTick(t *testing.T) {
	c, engines := toyCluster(t, 1)
	setConfig(t, c, map[string]string{"checkpoint.every_ticks": "5"})
	for range 3 {
		if err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	reads := configReads(c.Env.EventLog.(*eventlog.MemLog), 1)
	for tick := uint64(1); tick <= 3; tick++ {
		if reads[tick] != tick {
			t.Errorf("at tick %d the engine read the copy of tick %d", tick, reads[tick])
		}
	}
	if got := engines[0].tickConfig().Version(); got != 1 {
		t.Fatalf("the engine reads version %d, want 1", got)
	}
	if got := c.Config().Version(); got != 1 {
		t.Fatalf("the cluster says version %d, want 1", got)
	}
}

func TestAnEngineThatFindsNoConfigurationEngineReadsTheDefaultsAndTheLogSaysThereWasNone(
	t *testing.T,
) {
	env := newToyEnv(t)
	e := New(&toyState{id: 1}, env, 1)
	if err := e.Step(); err != nil {
		t.Fatal(err)
	}
	if reads := configReads(env.EventLog.(*eventlog.MemLog), 1); len(reads) != 1 || reads[1] != 0 {
		t.Fatalf("the log says %v, want the read of tick 1 with no copy", reads)
	}
	if e.tickConfig() != config.ReadOnlySnapshot(config.Defaults()) {
		t.Fatal("the engine does not read the defaults")
	}
}

func TestTheConfigurationEngineReadsItsOwnStateAndRecordsNoReadOfItself(t *testing.T) {
	c, _ := toyCluster(t)
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if reads := configReads(c.Env.EventLog.(*eventlog.MemLog), sim.ConfigStateID); len(reads) != 0 {
		t.Fatalf("the configuration engine recorded a read of itself: %v", reads)
	}
}
