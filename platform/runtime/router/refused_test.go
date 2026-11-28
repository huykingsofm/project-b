package router_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/runtime/router"
	"github.com/huykingsofm/project-b/platform/runtime/testkit"
	"github.com/huykingsofm/project-b/platform/sim"
)

func TestAMessageThatFindsTheChannelOfItsReceiverFullIsRefusedAndCounted(t *testing.T) {
	_, out := router.NewChannel(5, 1)
	rt := router.NewRouter()
	rt.Register(out)
	if reason, ok := rt.Deliver(testkit.ToyMessage(5, 1)); !ok {
		t.Fatalf("the first message was refused: %v", reason)
	}
	if reason, ok := rt.Deliver(testkit.ToyMessage(5, 2)); ok || reason != sim.DeliveryFailureFull {
		t.Fatalf("the second message: %v, %v, want it refused as full", reason, ok)
	}
	// an engine that is not there is another refusal, which is not counted here
	reason, ok := rt.Deliver(testkit.ToyMessage(9, 3))
	if ok || reason != sim.DeliveryFailureNotStarted {
		t.Fatalf("a message for an engine that is not there: %v, %v", reason, ok)
	}
	if rt.Refused() != 1 {
		t.Fatalf("%d messages were counted as refused, want 1", rt.Refused())
	}
}
