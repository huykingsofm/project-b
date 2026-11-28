package cluster_test

import (
	"strings"
	"testing"

	"github.com/huykingsofm/project-b/platform/runtime/testkit"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/logging"
)

func TestTheClusterLogsTheRefusedMessagesAsTheConfigurationSays(t *testing.T) {
	finish := testkit.RecordLogs(t)
	c, toys := testkit.NewToyCluster(t, 1)
	testkit.SetConfig(t, c, map[string]string{"router.log_every_refused_messages": "3"})
	// the queue of an engine of this environment holds 16: the other 7 are refused
	for i := range 23 {
		c.Env.Router.Deliver(testkit.ToyMessage(toys[0].StateID(), uint64(i+1)))
	}
	c.RunPeriodicTasks()
	c.RunPeriodicTasks() // nothing is new
	for i := range 2 {   // fewer than 3 more are not logged
		c.Env.Router.Deliver(testkit.ToyMessage(toys[0].StateID(), uint64(100+i)))
	}
	c.RunPeriodicTasks()
	var refused []logging.Entry
	for _, e := range finish() {
		if strings.Contains(e.Message, "refused") {
			refused = append(refused, e)
		}
	}
	if len(refused) != 1 || refused[0].Level != logging.LevelWarn {
		t.Fatalf("%d entries about refused messages, want one warning", len(refused))
	}
	if total, _ := testkit.LogField(refused[0], "total"); total != uint64(7) {
		t.Fatalf("the entry says %v messages were refused, want 7", total)
	}
}

func TestTheClusterLogsTheMessagesItLostAtOnceByDefault(t *testing.T) {
	finish := testkit.RecordLogs(t)
	c, _ := testkit.NewToyCluster(t, 1)
	// a message that comes back to a sender that is gone is lost
	gone := testkit.ToyMessage(1, 1).Stamp(sim.Header{From: 9})
	c.Env.Router.Return(gone, sim.DeliveryFailureRetired)
	c.RunPeriodicTasks()
	var lost int
	for _, e := range finish() {
		if strings.Contains(e.Message, "messages lost") {
			lost++
		}
	}
	if lost != 1 {
		t.Fatalf("%d entries about lost messages, want 1", lost)
	}
}
