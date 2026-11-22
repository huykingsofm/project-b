package engine

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
)

func TestWhatTheOutgoingChannelSendsTheIncomingChannelReadsAndAFullChannelRefuses(t *testing.T) {
	in, out := NewChannel(1, 1)
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

	r := ReturnedMessage{Message: m, Reason: sim.Full}
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
