package engine

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
)

// compensatedForwardAction sends an add to another engine, with a message that undoes it, for that
// engine to send to a third, when the first cannot be delivered.
type compensatedForwardAction struct {
	To, Back sim.StateID
	N        int64
}

func (compensatedForwardAction) ID() sim.ActionID { return 60010 }
func (compensatedForwardAction) Priority() int16  { return 0 }
func (compensatedForwardAction) Actor() any       { return nil }
func (a compensatedForwardAction) Apply(
	_ *sim.Context, _ *toyState, r *sim.Registry[*toyState], _ *sim.Plan[*toyState],
) {
	undo := sim.NewActionMessage[*toyState](a.Back, onfail.Drop{}, toyAction{N: -a.N})
	r.Send(a.To, onfail.Compensate{With: undo}, toyAction{N: a.N})
}
func (a compensatedForwardAction) Clone() sim.Action[*toyState] { return a }

// returnedCount is how many messages came back to the engine, as its event log says.
func returnedCount(c *Cluster, engine sim.StateID) int {
	n := 0
	for _, r := range c.Env.EventLog.(*aLogThatSaysWhatIsSafe).Records {
		if r.Kind() == eventlog.KindReturned && r.Engine == engine {
			n++
		}
	}
	return n
}

func stepAll(t *testing.T, engines []*Engine[*toyState], ticks int) {
	t.Helper()
	for range ticks {
		for _, e := range engines {
			if err := e.Step(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func threeToys(
	t *testing.T,
) (*Cluster, *Engine[*toyState], *Engine[*toyState], *Engine[*toyState]) {
	t.Helper()
	env := newToyEnv(t)
	env.EventLog = &aLogThatSaysWhatIsSafe{safe: map[sim.StateID]uint64{}}
	c := newCluster(t, env)
	a, b, d := New(&toyState{id: 1}, env, 1), New(&toyState{id: 2}, env, 1),
		New(&toyState{id: 3}, env, 1)
	c.Add(a)
	c.Add(b)
	c.Add(d)
	return c, a, b, d
}

func TestOnlyTheMessagesThatCameBackAreLetGoOfAndTheOnesForAnotherReceiverStay(t *testing.T) {
	_, a, b, _ := threeToys(t)
	held := &a.peer.data.unsettledMessages
	a.ScheduleAt(1, forwardAction{To: 2, N: 1})
	a.ScheduleAt(2, forwardAction{To: 2, N: 1})
	a.ScheduleAt(3, forwardAction{To: 3, N: 1})
	stepAll(t, []*Engine[*toyState]{a}, 3)
	if held.Len() != 3 {
		t.Fatalf("a holds %d messages, want 3", held.Len())
	}
	b.Remove()
	stepAll(t, []*Engine[*toyState]{a}, 1)
	if held.Len() != 1 || held.Sent()[0].Dest() != 3 {
		t.Fatalf("a holds %v: want only the message for engine 3", held.Sent())
	}
}

func TestACompensationRunsOnceWhenTheReceiverIsRemovedWhileTheMessageWaits(t *testing.T) {
	c, a, b, d := threeToys(t)
	first := uint64(unsettledResendFirst.Get(config.Defaults()))
	a.ScheduleAt(1, compensatedForwardAction{To: 2, Back: 3, N: 7})
	stepAll(t, []*Engine[*toyState]{a}, 1)
	b.Remove()
	// far past the tick at which it would be sent again
	stepAll(t, []*Engine[*toyState]{a, d}, int(first)+30)
	if d.State().Total != -7 {
		t.Fatalf("engine 3 has %d, want one compensation: -7", d.State().Total)
	}
	if got := returnedCount(c, 1); got != 1 {
		t.Fatalf("%d messages came back to a, want 1", got)
	}
}

func TestAFullReceiverRefusesAtTheSendAndTheMessageGoesOnceWithANewNumber(t *testing.T) {
	c, a, b, _ := threeToys(t)
	held := &a.peer.data.unsettledMessages
	size := c.Env.QueueSize
	for range size + 1 { // one more than b's channel holds
		a.ScheduleAt(1, forwardAction{To: 2, N: 1})
	}
	stepAll(t, []*Engine[*toyState]{a}, 1)
	if got := returnedCount(c, 1); got != 1 {
		t.Fatalf("%d messages came back at the send, want the 1 that did not fit", got)
	}
	if got := len(held.Sent()); got != size {
		t.Fatalf("a holds %d sent messages, want %d: the refused one never went", got, size)
	}
	stepAll(t, []*Engine[*toyState]{b, a, b}, 1) // b makes room, a sends the refused one again
	if b.State().Total != int64(size)+1 {
		t.Fatalf("b has %d, want %d: every message once", b.State().Total, size+1)
	}
}

func TestAMessageForAnEngineThatHasNotStartedIsSentAgainTheMostTimesAndThenDropped(t *testing.T) {
	c, a, _, _ := threeToys(t)
	held := &a.peer.data.unsettledMessages
	most := sim.MaxAttempts.Get(config.Defaults())
	a.ScheduleAt(1, forwardAction{To: 99, N: 1})
	stepAll(t, []*Engine[*toyState]{a}, most+10)
	// it is sent again `most` times, so it is tried once more than that
	if got := returnedCount(c, 1); got != most+1 {
		t.Fatalf("it came back %d times, want %d: the first try and %d tries again",
			got, most+1, most)
	}
	if held.Len() != 0 || len(held.Unsent()) != 0 {
		t.Fatalf("a still holds %d messages", held.Len())
	}
}

func TestAnAckThatComesBeforeAMessageIsDueStopsItFromBeingSentAgain(t *testing.T) {
	c, a, b, _ := threeToys(t)
	held := &a.peer.data.unsettledMessages
	first := uint64(unsettledResendFirst.Get(config.Defaults()))
	a.ScheduleAt(1, forwardAction{To: 2, N: 1})
	stepAll(t, []*Engine[*toyState]{a}, 1)
	c.Env.Acks.Raise(2, 1, held.Sent()[0].Head().Seq)
	stepAll(t, []*Engine[*toyState]{a}, 1)
	if held.Len() != 0 {
		t.Fatalf("a holds %d messages after the ack, want none", held.Len())
	}
	stepAll(t, []*Engine[*toyState]{a}, int(first)+5)
	if got := len(b.peer.in.In); got != 1 {
		t.Fatalf("%d messages wait for b, want the first only: an acked message is not sent again",
			got)
	}
}

func TestAMessageThatFindsTheChannelFullWhenItIsSentAgainWaitsAndIsNotReturned(t *testing.T) {
	c, a, b, _ := threeToys(t)
	held := &a.peer.data.unsettledMessages
	first := uint64(unsettledResendFirst.Get(config.Defaults()))
	size := c.Env.QueueSize
	a.ScheduleAt(1, forwardAction{To: 2, N: 1})
	for range size - 1 { // the channel of b is full, with the first message among them
		a.ScheduleAt(2, forwardAction{To: 2, N: 1})
	}
	stepAll(t, []*Engine[*toyState]{a}, int(first)+3)
	if got := returnedCount(c, 1); got != 0 {
		t.Fatalf("%d messages came back, want none: a full channel makes a copy wait", got)
	}
	if held.Len() != size {
		t.Fatalf("a holds %d messages, want %d", held.Len(), size)
	}
	stepAll(t, []*Engine[*toyState]{b}, 1)
	if b.State().Total != int64(size) {
		t.Fatalf("b has %d, want %d", b.State().Total, size)
	}
	stepAll(t, []*Engine[*toyState]{a, b}, 250) // every copy that is sent again is dropped by b
	if b.State().Total != int64(size) {
		t.Fatalf("b has %d after copies came, want %d: a copy is taken once", b.State().Total, size)
	}
}

func TestACheckpointTakenAfterAMessageCameBackHoldsNoMessageForItAndAnEarlierOneDoes(t *testing.T) {
	_, a, b, _ := threeToys(t)
	a.ScheduleAt(1, forwardAction{To: 2, N: 1})
	stepAll(t, []*Engine[*toyState]{a}, 1)
	before := a.Checkpoint()
	b.Remove()
	stepAll(t, []*Engine[*toyState]{a}, 1)
	after := a.Checkpoint()
	if got := before.Peer.unsettledMessages.Len(); got != 1 {
		t.Fatalf("the checkpoint before holds %d messages, want 1", got)
	}
	if got := after.Peer.unsettledMessages.Len(); got != 0 {
		t.Fatalf("the checkpoint after holds %d messages, want none", got)
	}
}

func TestACompensationRunsOnceWhenTheMessageWasSentAgainBeforeItsReceiverWasRemoved(t *testing.T) {
	c, a, b, d := threeToys(t)
	first := uint64(unsettledResendFirst.Get(config.Defaults()))
	a.ScheduleAt(1, compensatedForwardAction{To: 2, Back: 3, N: 7})
	// b takes nothing: the message is sent again, and two copies wait in its channel
	stepAll(t, []*Engine[*toyState]{a}, int(first)+2)
	if got := len(b.peer.in.In); got != 2 {
		t.Fatalf("%d copies wait for b, want 2", got)
	}
	b.Remove()
	stepAll(t, []*Engine[*toyState]{a, d}, int(first)+30)
	if d.State().Total != -7 {
		t.Fatalf("engine 3 has %d, want one compensation: -7", d.State().Total)
	}
	if got := returnedCount(c, 1); got != 1 {
		t.Fatalf("%d messages came back to a, want 1: a copy is not another message", got)
	}
}

func TestACopyOfAMessageThatWasTakenIsReturnedOnceWhenItsReceiverIsRemovedAndItsOnFailRunsOnce(
	t *testing.T) {
	c, a, b, d := threeToys(t)
	first := uint64(unsettledResendFirst.Get(config.Defaults()))
	a.ScheduleAt(1, compensatedForwardAction{To: 2, Back: 3, N: 7})
	stepAll(t, []*Engine[*toyState]{a, b}, 1) // b takes the message
	if b.State().Total != 7 {
		t.Fatalf("b has %d, want 7", b.State().Total)
	}
	stepAll(t, []*Engine[*toyState]{a}, int(first)+1) // no ack: a sends it again
	b.Remove()                                        // the copy goes back to a
	stepAll(t, []*Engine[*toyState]{a, d}, int(first)+30)
	// the state of b is gone with it, so what the message did is gone: its sender undoes it, once
	if d.State().Total != -7 {
		t.Fatalf("engine 3 has %d, want one compensation: -7", d.State().Total)
	}
	if got := returnedCount(c, 1); got != 1 {
		t.Fatalf("%d messages came back to a, want 1", got)
	}
}
