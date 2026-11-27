package engine_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
)

func TestWhatTheOutgoingChannelSendsTheIncomingChannelReadsAndAFullChannelRefuses(t *testing.T) {
	in, out := engine.NewChannel(1, 1)
	if out.StateID() != 1 {
		t.Fatalf("id = %v", out.StateID())
	}
	m := sim.NewActionMessage[*toyState](1, onfail.Drop{})
	if !out.Send(m) {
		t.Fatal("the first message was refused")
	}
	if out.Send(m) {
		t.Fatal("a full channel took a message")
	}
	if got := <-in.In; got != m {
		t.Fatal("the incoming channel did not give what was sent")
	}

	r := engine.ReturnedMessage{Message: m, Reason: sim.DeliveryFailureFull}
	if !out.SendBack(r) {
		t.Fatal("the returned message was refused")
	}
	if out.SendBack(r) {
		t.Fatal("a full back channel took a message")
	}
	if got := <-in.Back; got != r {
		t.Fatal("the incoming channel did not give what came back")
	}
}

func TestARouterSaysAnIDWithNoEngineIsNotStartedUntilItsEngineIsRetired(t *testing.T) {
	message := func(to sim.StateID) sim.Message {
		return sim.NewActionMessage(to, onfail.Drop{}, toyAction{N: 1})
	}
	_, out := engine.NewChannel(5, 4)
	rt := engine.NewRouter()
	if reason, ok := rt.Deliver(message(5)); ok || reason != sim.DeliveryFailureNotStarted {
		t.Fatalf("an ID with no engine bounced as %v, %v", reason, ok)
	}
	rt.Register(out)
	rt.Retire(5)
	for to, want := range map[sim.StateID]sim.DeliveryFailure{
		5: sim.DeliveryFailureRetired,
		6: sim.DeliveryFailureNotStarted,
	} {
		if reason, ok := rt.Deliver(message(to)); ok || reason != want {
			t.Errorf("a message to %d bounced as %v, %v; want %v", to, reason, ok, want)
		}
	}
}
