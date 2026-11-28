package cluster_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/runtime/cluster/stepping"
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/runtime/testkit"
	"github.com/huykingsofm/project-b/platform/sim"
)

// configReads returns, for each tick of the engine, the tick of the copy of the configuration
// engine that it read, as its log says.
func configReads(mem *eventlog.MemLog, engine sim.StateID) map[uint64]uint64 {
	out := map[uint64]uint64{}
	for _, r := range mem.Records {
		if read, ok := r.Body.(eventlog.StableRead); ok && r.Engine == engine &&
			read.Source == sim.CoordinatorStateID {
			out[r.Tick] = read.CopyTick
		}
	}
	return out
}

func TestAnEngineReadsTheConfigurationFromTheCopyOfTheConfigurationEngineAtEveryTick(t *testing.T) {
	c, _ := testkit.NewToyCluster(t, 1)
	testkit.SetConfig(t, c, map[string]string{"checkpoint.every_ticks": "5"})
	for range 3 {
		if err := stepping.Step(c); err != nil {
			t.Fatal(err)
		}
	}
	reads := configReads(c.Env.EventLog.(*eventlog.MemLog), 1)
	for tick := uint64(1); tick <= 3; tick++ {
		if reads[tick] != tick {
			t.Errorf("at tick %d the engine read the copy of tick %d", tick, reads[tick])
		}
	}
	snapshot := c.CoordinatorEngine().State().Snapshot()
	if got, ok := configText(snapshot, "checkpoint.every_ticks"); !ok ||
		got != "5" {
		t.Fatalf("the coordinator engine holds %q ticks between checkpoints, want 5", got)
	}
}

func TestAnEngineThatFindsNoConfigurationEngineReadsTheDefaultsAndTheLogSaysThereWasNone(
	t *testing.T,
) {
	env := testkit.NewToyEnv(t)
	e := engine.NewEngine(&testkit.ToyState{}, 1, env)
	if err := e.Step(); err != nil {
		t.Fatal(err)
	}
	if reads := configReads(env.EventLog.(*eventlog.MemLog), 1); len(reads) != 1 || reads[1] != 0 {
		t.Fatalf("the log says %v, want the read of tick 1 with no copy", reads)
	}
}

func TestTheConfigurationEngineReadsTheCopyOfItsLastTickLikeAnyOtherEngine(t *testing.T) {
	c, _ := testkit.NewToyCluster(t, 0)
	for range 3 {
		if err := stepping.Step(c); err != nil {
			t.Fatal(err)
		}
	}
	reads := configReads(c.Env.EventLog.(*eventlog.MemLog), sim.CoordinatorStateID)
	if reads[1] != 0 || reads[2] != 1 || reads[3] != 2 {
		t.Fatalf("the coordinator engine read the copies of the ticks %v", reads)
	}
}

func TestTheClusterSendsTheConfigurationEngineTheDocumentOfItsStoreOnceAndOnlyWhatChanges(
	t *testing.T,
) {
	c, _ := testkit.NewToyCluster(t, 1)
	commands := func() int {
		n := 0
		for _, r := range c.Env.EventLog.(*eventlog.MemLog).Records {
			if _, ok := r.Body.(eventlog.Command); ok && r.Engine == sim.CoordinatorStateID {
				n++
			}
		}
		return n
	}
	if err := stepping.Step(c); err != nil || commands() != 0 {
		t.Fatalf("a store that took no document sent %d commands (%v)", commands(), err)
	}
	testkit.SetConfig(t, c, map[string]string{"checkpoint.every_ticks": "5"})
	for range 2 {
		if err := stepping.Step(c); err != nil {
			t.Fatal(err)
		}
	}
	if commands() != 1 {
		t.Fatalf("%d commands after a document, want 1", commands())
	}
	testkit.SetConfig(t, c, map[string]string{"checkpoint.every_ticks": "5"})
	for range 2 {
		if err := stepping.Step(c); err != nil {
			t.Fatal(err)
		}
	}
	if commands() != 1 {
		t.Fatalf("the same document sent a command: %d, want 1", commands())
	}
	testkit.SetConfig(t, c, map[string]string{})
	for range 2 {
		if err := stepping.Step(c); err != nil {
			t.Fatal(err)
		}
	}
	_, set := configText(c.CoordinatorEngine().State().Snapshot(), "checkpoint.every_ticks")
	if commands() != 2 || set {
		t.Fatalf("an empty document: %d commands, the default not back", commands())
	}
}

func TestAStoreRefusesADocumentWithAWrongValueAndTheClusterSendsNothing(t *testing.T) {
	c, _ := testkit.NewToyCluster(t, 1)
	if err := c.ConfigStore().Apply(map[string]string{"action.max_depth": "99"}); err == nil {
		t.Fatal("a value above its limit was taken")
	}
	if cur := c.ConfigStore().Current(); cur.Generation != 0 {
		t.Fatalf("a refused document made generation %d", cur.Generation)
	}
}

// configText is the text that the snapshot holds for the variable, and whether it holds one: a
// variable that was never set has none.
func configText(s config.ReadOnlySnapshot, name string) (string, bool) {
	for key, text := range s.(*config.Snapshot).Texts() {
		if key == name {
			return text, true
		}
	}
	return "", false
}
