package engine

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/onfail"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
)

func TestWhatTheOutgoingChannelSendsTheIncomingChannelReadsAndAFullChannelRefuses(t *testing.T) {
	in, out := NewChannel(message.ZoneAddress(1), 1)
	if out.Address() != message.ZoneAddress(1) {
		t.Fatalf("address = %v", out.Address())
	}
	m := action.MessageToZone(1, onfail.Drop{}, nil...)
	if !out.Send(m) {
		t.Fatal("the first message was refused")
	}
	if out.Send(m) {
		t.Fatal("a full channel took a message")
	}
	if got := <-in.In; got != m {
		t.Fatal("the incoming channel did not give what was sent")
	}

	r := ReturnedMessage{Message: m, Reason: message.Full}
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
