package engine_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/runtime/cluster/stepping"
	"github.com/huykingsofm/project-b/platform/runtime/testkit"
)

func TestAnEngineLogsHowLongItsTicksTookEveryNumberOfTicksTheConfigurationSays(t *testing.T) {
	finish := testkit.RecordLogs(t)
	c, toys := testkit.NewToyCluster(t, 1)
	testkit.SetConfig(t, c, map[string]string{"engine.log_tick_times_every_ticks": "5"})
	for range 16 {
		if err := stepping.Step(c); err != nil {
			t.Fatal(err)
		}
	}
	var logged int
	for _, e := range finish() {
		if id, _ := testkit.LogField(e, "engine"); id != toys[0].StateID() {
			continue
		}
		logged++
		if ticks, _ := testkit.LogField(e, "ticks"); ticks != uint64(5) {
			t.Fatalf("the entry counts %v ticks, want 5", ticks)
		}
		if _, ok := testkit.LogField(e, "p99"); !ok {
			t.Fatal("the entry has no time of the slowest ticks")
		}
	}
	if logged != 3 { // the ticks 5, 10 and 15 of 16
		t.Fatalf("%d entries for the engine, want 3", logged)
	}
}
