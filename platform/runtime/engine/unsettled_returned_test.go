package engine_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/runtime/cluster"
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/runtime/testkit"
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
	_ *sim.Context, _ *testkit.ToyState, r *sim.ActionRegistry[*testkit.ToyState],
	_ *sim.Plan[*testkit.ToyState],
) {
	undo := sim.NewActionMessage[*testkit.ToyState](
		a.Back, onfail.Drop{}, testkit.ToyAction{N: -a.N},
	)
	r.Send(a.To, onfail.Compensate{With: undo}, testkit.ToyAction{N: a.N})
}
func (a compensatedForwardAction) Clone() sim.Action[*testkit.ToyState] { return a }

// returnedCount is how many messages came back to the engine, as its event log says.
func returnedCount(c *cluster.Cluster, engine sim.StateID) int {
	n := 0
	for _, r := range c.Env.EventLog.(*aLogThatSaysWhatIsSafe).Records {
		if r.Kind() == eventlog.KindReturned && r.Engine == engine {
			n++
		}
	}
	return n
}

// waitingFor is how many messages wait in the channel of the engine, which takes none: it is found
// by sending messages that cost nothing until the channel refuses one, and it is full after it.
func waitingFor(t *testing.T, c *cluster.Cluster, id sim.StateID) int {
	t.Helper()
	size := c.Env.QueueSize
	for accepted := range size + 1 {
		probe := sim.NewActionMessage[*testkit.ToyState](id, onfail.Drop{}, testkit.ToyAction{}).
			Stamp(sim.Header{From: 98, Seq: uint64(accepted) + 1})
		if _, ok := c.Env.Router.Deliver(probe); !ok {
			return size - accepted
		}
	}
	t.Fatal("the channel of the engine took more messages than it holds")
	return 0
}

func stepAll(t *testing.T, engines []*engine.Engine[*testkit.ToyState], ticks int) {
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
) (
	*cluster.Cluster, *engine.Engine[*testkit.ToyState], *engine.Engine[*testkit.ToyState],
	*engine.Engine[*testkit.ToyState],
) {
	t.Helper()
	env := testkit.NewToyEnv(t)
	env.EventLog = &aLogThatSaysWhatIsSafe{safe: map[sim.StateID]uint64{}}
	c := testkit.NewCluster(t, env)
	engines := testkit.AddToys(t, c, 3)
	return c, engines[0], engines[1], engines[2]
}

func TestOnlyTheMessagesThatCameBackAreLetGoOfAndTheOnesForAnotherReceiverStay(t *testing.T) {
	_, a, b, _ := threeToys(t)
	a.ScheduleAt(1, testkit.ForwardAction{To: 2, N: 1})
	a.ScheduleAt(2, testkit.ForwardAction{To: 2, N: 1})
	a.ScheduleAt(3, testkit.ForwardAction{To: 3, N: 1})
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a}, 3)
	if len(a.GetSentMessages()) != 3 {
		t.Fatalf("a holds %d messages, want 3", len(a.GetSentMessages()))
	}
	b.Retire()
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a}, 1)
	if len(a.GetSentMessages()) != 1 || a.GetSentMessages()[0].Dest() != 3 {
		t.Fatalf("a holds %v: want only the message for engine 3", a.GetSentMessages())
	}
}

func TestACompensationRunsOnceWhenTheReceiverIsRetiredWhileTheMessageWaits(t *testing.T) {
	c, a, b, d := threeToys(t)
	first := uint64(resendFirstTicks)
	a.ScheduleAt(1, compensatedForwardAction{To: 2, Back: 3, N: 7})
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a}, 1)
	b.Retire()
	// far past the tick at which it would be sent again
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a, d}, int(first)+30)
	if d.State().Total != -7 {
		t.Fatalf("engine 3 has %d, want one compensation: -7", d.State().Total)
	}
	if got := returnedCount(c, 1); got != 1 {
		t.Fatalf("%d messages came back to a, want 1", got)
	}
}

func TestAFullReceiverRefusesAtTheSendAndTheMessageGoesOnceWithANewNumber(t *testing.T) {
	c, a, b, _ := threeToys(t)
	size := c.Env.QueueSize
	for range size + 1 { // one more than b's channel holds
		a.ScheduleAt(1, testkit.ForwardAction{To: 2, N: 1})
	}
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a}, 1)
	if got := returnedCount(c, 1); got != 1 {
		t.Fatalf("%d messages came back at the send, want the 1 that did not fit", got)
	}
	if got := len(a.GetSentMessages()); got != size {
		t.Fatalf("a holds %d sent messages, want %d: the refused one never went", got, size)
	}
	stepAll(t, []*engine.Engine[*testkit.ToyState]{b, a, b},
		1) // b makes room, a sends the refused one again
	if b.State().Total != int64(size)+1 {
		t.Fatalf("b has %d, want %d: every message once", b.State().Total, size+1)
	}
}

func TestAMessageForAnEngineThatHasNotStartedIsSentAgainTheMostTimesAndThenDropped(t *testing.T) {
	c, a, _, _ := threeToys(t)
	most := sim.MaxAttempts.Get(config.Defaults())
	a.ScheduleAt(1, testkit.ForwardAction{To: 99, N: 1})
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a}, most+10)
	// it is sent again `most` times, so it is tried once more than that
	if got := returnedCount(c, 1); got != most+1 {
		t.Fatalf("it came back %d times, want %d: the first try and %d tries again",
			got, most+1, most)
	}
	if len(a.GetSentMessages()) != 0 {
		t.Fatalf("a still holds %d messages", len(a.GetSentMessages()))
	}
}

func TestAnAckThatComesBeforeAMessageIsDueStopsItFromBeingSentAgain(t *testing.T) {
	c, a, _, _ := threeToys(t)
	first := uint64(resendFirstTicks)
	a.ScheduleAt(1, testkit.ForwardAction{To: 2, N: 1})
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a}, 1)
	c.Env.AckBoard.Raise(2, 1, a.GetSentMessages()[0].Head().Seq)
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a}, 1)
	if len(a.GetSentMessages()) != 0 {
		t.Fatalf("a holds %d messages after the ack, want none", len(a.GetSentMessages()))
	}
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a}, int(first)+5)
	if got := waitingFor(t, c, 2); got != 1 {
		t.Fatalf("%d messages wait for b, want the first only: an acked message is not sent again",
			got)
	}
}

func TestAMessageThatFindsTheChannelFullWhenItIsSentAgainWaitsAndIsNotReturned(t *testing.T) {
	c, a, b, _ := threeToys(t)
	first := uint64(resendFirstTicks)
	size := c.Env.QueueSize
	a.ScheduleAt(1, testkit.ForwardAction{To: 2, N: 1})
	for range size - 1 { // the channel of b is full, with the first message among them
		a.ScheduleAt(2, testkit.ForwardAction{To: 2, N: 1})
	}
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a}, int(first)+3)
	if got := returnedCount(c, 1); got != 0 {
		t.Fatalf("%d messages came back, want none: a full channel makes a copy wait", got)
	}
	if len(a.GetSentMessages()) != size {
		t.Fatalf("a holds %d messages, want %d", len(a.GetSentMessages()), size)
	}
	stepAll(t, []*engine.Engine[*testkit.ToyState]{b}, 1)
	if b.State().Total != int64(size) {
		t.Fatalf("b has %d, want %d", b.State().Total, size)
	}
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a, b},
		250) // every copy that is sent again is dropped by b
	if b.State().Total != int64(size) {
		t.Fatalf("b has %d after copies came, want %d: a copy is taken once", b.State().Total, size)
	}
}

func TestACheckpointTakenAfterAMessageCameBackHoldsNoMessageForItAndAnEarlierOneDoes(t *testing.T) {
	_, a, b, _ := threeToys(t)
	a.ScheduleAt(1, testkit.ForwardAction{To: 2, N: 1})
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a}, 1)
	before, err := a.Checkpoint().Encode()
	if err != nil {
		t.Fatal(err)
	}
	b.Retire()
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a}, 1)
	after, err := a.Checkpoint().Encode()
	if err != nil {
		t.Fatal(err)
	}
	for data, want := range map[string]int{string(before): 1, string(after): 0} {
		r, err := restoreEngine[*testkit.ToyState]([]byte(data), testkit.NewToyEnv(t))
		if err != nil {
			t.Fatal(err)
		}
		if got := len(r.GetSentMessages()); got != want {
			t.Fatalf("a checkpoint holds %d messages, want %d", got, want)
		}
	}
}

func TestACompensationRunsOnceWhenTheMessageWasSentAgainBeforeItsReceiverWasRetired(t *testing.T) {
	c, a, b, d := threeToys(t)
	first := uint64(resendFirstTicks)
	a.ScheduleAt(1, compensatedForwardAction{To: 2, Back: 3, N: 7})
	// b takes nothing: the message is sent again, and two copies wait in its channel
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a}, int(first)+2)
	if got := waitingFor(t, c, 2); got != 2 {
		t.Fatalf("%d copies wait for b, want 2", got)
	}
	b.Retire()
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a, d}, int(first)+30)
	if d.State().Total != -7 {
		t.Fatalf("engine 3 has %d, want one compensation: -7", d.State().Total)
	}
	if got := returnedCount(c, 1); got != 1 {
		t.Fatalf("%d messages came back to a, want 1: a copy is not another message", got)
	}
}

func TestACopyOfAMessageThatWasTakenIsReturnedOnceWhenItsReceiverIsRetiredAndItsOnFailRunsOnce(
	t *testing.T) {
	c, a, b, d := threeToys(t)
	first := uint64(resendFirstTicks)
	a.ScheduleAt(1, compensatedForwardAction{To: 2, Back: 3, N: 7})
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a, b}, 1) // b takes the message
	if b.State().Total != 7 {
		t.Fatalf("b has %d, want 7", b.State().Total)
	}
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a}, int(first)+1) // no ack: a sends it again
	b.Retire()                                                       // the copy goes back to a
	stepAll(t, []*engine.Engine[*testkit.ToyState]{a, d}, int(first)+30)
	// the state of b is gone with it, so what the message did is gone: its sender undoes it, once
	if d.State().Total != -7 {
		t.Fatalf("engine 3 has %d, want one compensation: -7", d.State().Total)
	}
	if got := returnedCount(c, 1); got != 1 {
		t.Fatalf("%d messages came back to a, want 1", got)
	}
}
