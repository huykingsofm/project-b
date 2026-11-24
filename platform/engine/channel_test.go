package engine

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/shared/ids"
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

	r := ReturnedMessage{Message: m, Reason: sim.DeliveryFailureFull}
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

// aDirectory says what each ID is, for the tests of a router that asks one.
type aDirectory map[sim.StateID]ids.Status

func (d aDirectory) StateStatus(id sim.StateID) ids.Status {
	if status, ok := d[id]; ok {
		return status
	}
	return ids.NotGiven
}

func TestARouterSaysWhyAnIDHasNoEngineByWhatItsDirectorySays(t *testing.T) {
	message := func(to sim.StateID) sim.Message {
		return sim.NewActionMessage[*toyState](to, onfail.Drop{}, &toyAction{N: 1})
	}
	rt := NewRouter()
	if reason, ok := rt.Deliver(message(5)); ok || reason != sim.DeliveryFailureNotStarted {
		t.Fatalf("with no directory an ID with no engine bounced as %v, %v", reason, ok)
	}
	rt.SetDirectory(aDirectory{
		5: ids.Live, 6: ids.InGrave, // 7 is not given
	})
	for to, want := range map[sim.StateID]sim.DeliveryFailure{
		5: sim.DeliveryFailureNotStarted,
		6: sim.DeliveryFailureRetired,
		7: sim.DeliveryFailureNotGiven,
	} {
		if reason, ok := rt.Deliver(message(to)); ok || reason != want {
			t.Errorf("a message to %d bounced as %v, %v; want %v", to, reason, ok, want)
		}
	}
}

func TestARouterWithADirectoryRemembersNoEngineThatWasRetired(t *testing.T) {
	_, out := NewChannel(5, 4)
	rt := NewRouter()
	rt.SetDirectory(aDirectory{5: ids.InGrave})
	rt.Register(out)
	rt.Retire(5)
	if got := len(rt.data.Load().retired); got != 0 {
		t.Fatalf("the router remembers %d engines that were retired", got)
	}
	other := NewRouter() // no directory: it remembers
	other.Register(out)
	other.Retire(5)
	reason, _ := other.Deliver(sim.NewActionMessage[*toyState](5, onfail.Drop{}))
	if reason != sim.DeliveryFailureRetired {
		t.Fatalf("a message to an engine that was retired bounced as %v", reason)
	}
}
