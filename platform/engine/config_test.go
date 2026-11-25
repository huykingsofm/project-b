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
	if got := checkpointEvery.Get(engines[0].stateRegistry.Config()); got != 5 {
		t.Fatalf("the engine reads %d ticks between checkpoints, want 5", got)
	}
	if got := checkpointEvery.Get(c.ConfigEngine().State().Snapshot()); got != 5 {
		t.Fatalf("the configuration engine holds %d ticks between checkpoints, want 5", got)
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
	if e.stateRegistry.Config() != config.ReadOnlySnapshot(config.Defaults()) {
		t.Fatal("the engine does not read the defaults")
	}
}

func TestTheConfigurationEngineReadsTheCopyOfItsLastTickLikeAnyOtherEngine(t *testing.T) {
	c, _ := toyCluster(t)
	for range 3 {
		if err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	reads := configReads(c.Env.EventLog.(*eventlog.MemLog), sim.ConfigStateID)
	if reads[1] != 0 || reads[2] != 1 || reads[3] != 2 {
		t.Fatalf("the configuration engine read the copies of the ticks %v", reads)
	}
}

func TestTheClusterSendsTheConfigurationEngineTheDocumentOfItsStoreOnceAndOnlyWhatChanges(
	t *testing.T,
) {
	c, _ := toyCluster(t, 1)
	commands := func() int {
		n := 0
		for _, r := range c.Env.EventLog.(*eventlog.MemLog).Records {
			if _, ok := r.Body.(eventlog.Command); ok && r.Engine == sim.ConfigStateID {
				n++
			}
		}
		return n
	}
	if err := c.Step(); err != nil || commands() != 0 {
		t.Fatalf("a store that took no document sent %d commands (%v)", commands(), err)
	}
	setConfig(t, c, map[string]string{"checkpoint.every_ticks": "5"})
	for range 2 {
		if err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if commands() != 1 {
		t.Fatalf("%d commands after a document, want 1", commands())
	}
	setConfig(t, c, map[string]string{"checkpoint.every_ticks": "5"})
	for range 2 {
		if err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if commands() != 1 {
		t.Fatalf("the same document sent a command: %d, want 1", commands())
	}
	setConfig(t, c, map[string]string{})
	for range 2 {
		if err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if commands() != 2 || checkpointEvery.Get(c.ConfigEngine().State().Snapshot()) != 600 {
		t.Fatalf("an empty document: %d commands, the default not back", commands())
	}
}

func TestAStoreRefusesADocumentWithAWrongValueAndTheClusterSendsNothing(t *testing.T) {
	c, _ := toyCluster(t, 1)
	if err := c.ConfigStore().Apply(map[string]string{"action.max_depth": "99"}); err == nil {
		t.Fatal("a value above its limit was taken")
	}
	if cur := c.ConfigStore().Current(); cur.Generation != 0 {
		t.Fatalf("a refused document made generation %d", cur.Generation)
	}
}
