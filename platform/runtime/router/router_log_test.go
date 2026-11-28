package router_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/runtime/router"
	"github.com/huykingsofm/project-b/platform/runtime/testkit"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/shared/logging"
)

func TestReturnLogsAWarningWhenTheSenderIsGone(t *testing.T) {
	finish := testkit.RecordLogs(t)
	rt := router.NewRouter()
	m := sim.NewActionMessage[*testkit.ToyState](1, onfail.Drop{}).Stamp(sim.Header{From: 9})
	rt.Return(m, sim.DeliveryFailureRetired)
	entries := finish()

	if rt.Dropped() != 1 {
		t.Fatalf("dropped %d, want 1", rt.Dropped())
	}
	if len(entries) != 1 || entries[0].Level != logging.LevelWarn {
		t.Fatalf("entries %+v, want one warning", entries)
	}
}
