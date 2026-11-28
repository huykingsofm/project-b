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

	if len(entries) != 1 || entries[0].Level != logging.LevelWarn {
		t.Fatalf("entries %+v, want one warning", entries)
	}
}

func TestAMessageThatFindsTheChannelOfItsReceiverFullIsRefusedAndLogged(t *testing.T) {
	finish := testkit.RecordLogs(t)
	_, out := router.NewChannel(5, 1)
	rt := router.NewRouter()
	rt.Register(out)
	if reason, ok := rt.Deliver(testkit.ToyMessage(5, 1)); !ok {
		t.Fatalf("the first message was refused: %v", reason)
	}
	if reason, ok := rt.Deliver(testkit.ToyMessage(5, 2)); ok || reason != sim.DeliveryFailureFull {
		t.Fatalf("the second message: %v, %v, want it refused as full", reason, ok)
	}
	// an engine that is not there is another refusal, which is not logged here
	reason, ok := rt.Deliver(testkit.ToyMessage(9, 3))
	if ok || reason != sim.DeliveryFailureNotStarted {
		t.Fatalf("a message for an engine that is not there: %v, %v", reason, ok)
	}
	entries := finish()

	if len(entries) != 1 || entries[0].Level != logging.LevelWarn {
		t.Fatalf("entries %+v, want one warning, for the message refused as full", entries)
	}
	if dest, _ := testkit.LogField(entries[0], "dest"); dest != sim.StateID(5) {
		t.Fatalf("the warning says the message was for %v, want 5", dest)
	}
}
