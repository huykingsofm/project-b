package engine

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

func toMessage(to sim.StateID, seq uint64) sim.Message {
	return sim.NewActionMessage(to, onfail.Drop{}, &toyAction{N: 1}).
		Stamp(sim.Header{From: 1, Seq: seq, Tick: seq})
}

func TestAMessageThatWentWaitsForItsAckAndIsLetGoOfByTheNumberOfItsReceiver(t *testing.T) {
	var o unsettledMessages
	cfg := config.Defaults()
	o.MarkSent(toMessage(2, 1), 5, cfg)
	o.MarkSent(toMessage(3, 2), 5, cfg)
	o.MarkSent(toMessage(2, 3), 5, cfg)
	o.MarkSent(toMessage(2, 4), 5, cfg)
	if got := o.Receivers(); len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("Receivers = %v, want 2 and 3 once each, in order", got)
	}
	o.Ack(2, 3)
	if got := o.Sent(); len(got) != 2 || got[0].Head().Seq != 2 || got[1].Head().Seq != 4 {
		t.Fatalf("after an ack of 2 up to 3 the messages are %v: want numbers 2 and 4", got)
	}
	o.Ack(3, 1)  // not up to its message
	o.Ack(9, 99) // a receiver it has no message for
	if o.Len() != 2 {
		t.Fatalf("Len = %d, want 2", o.Len())
	}
}

func TestAMessageThatWentIsSentAgainAfterAWaitThatGrowsByTheStepUpToTheMost(t *testing.T) {
	var o unsettledMessages
	cfg := config.Defaults()
	first := uint64(unsettledResendFirst.Get(cfg))
	step := uint64(unsettledResendStep.Get(cfg))
	most := uint64(unsettledResendMost.Get(cfg))
	o.MarkSent(toMessage(2, 1), 100, cfg)
	if len(o.Due(100+first-1)) != 0 || len(o.Due(100+first)) != 1 {
		t.Fatalf("it is not due at %d and due at %d", 100+first-1, 100+first)
	}
	now := 100 + first
	want := first
	for range 40 { // far more than it takes to reach the most
		o.Reschedule(0, now, cfg)
		want = min(want+step, most)
		if len(o.Due(now+want-1)) != 0 || len(o.Due(now+want)) != 1 {
			t.Fatalf("the next wait is not %d ticks", want)
		}
		now += want
	}
	if want != most {
		t.Fatalf("the wait stopped at %d, want the most, %d", want, most)
	}
}

func TestRemoveLetsGoOfTheMessagesAtTheIndexesAndKeepsTheOrderOfTheRest(t *testing.T) {
	var o unsettledMessages
	cfg := config.Defaults()
	for seq := uint64(1); seq <= 5; seq++ {
		o.MarkSent(toMessage(2, seq), 0, cfg)
	}
	o.Remove([]int{1, 3})
	got := o.Sent()
	if len(got) != 3 || got[0].Head().Seq != 1 || got[1].Head().Seq != 3 || got[2].Head().Seq != 5 {
		t.Fatalf("the messages are %v", got)
	}
}

func TestUnsettledMessagesComeBackFromTheirBytesAndTheOnesThatWentAreSentAgain(t *testing.T) {
	codecs, err := toyCodecs()
	if err != nil {
		t.Fatal(err)
	}
	var o unsettledMessages
	o.AddUnsent(sim.NewActionMessage(2, onfail.Retry{}, &toyAction{N: 3}))
	o.MarkSent(toMessage(2, 7), 40, config.Defaults())
	var w binary.Writer
	if err := o.Encode(&w); err != nil {
		t.Fatal(err)
	}
	r := binary.NewReader(w.B)
	got, err := decodeUnsettledMessages(codecs, r, 1)
	if err != nil || r.Left() != 0 {
		t.Fatalf("error %v, %d bytes left", err, r.Left())
	}
	if len(got.Unsent()) != 1 || len(got.Sent()) != 1 || got.Sent()[0].Head().Seq != 7 {
		t.Fatalf("it came back as %v and %v", got.Unsent(), got.Sent())
	}
	if len(got.Due(0)) != 1 {
		t.Fatal("a message that went is not sent again at once after a restore")
	}
	for n := range w.B { // every proper prefix
		if _, err := decodeUnsettledMessages(codecs, binary.NewReader(w.B[:n]), 1); err == nil {
			t.Fatalf("%d of %d bytes were read", n, len(w.B))
		}
	}
	var withoutSent binary.Writer
	held := o.withoutSent()
	if err := held.Encode(&withoutSent); err != nil || len(withoutSent.B) >= len(w.B) {
		t.Fatalf("what the hash is given is not smaller: %d and %d, %v",
			len(withoutSent.B), len(w.B), err)
	}
}

func TestTheBoardOnlyGoesUp(t *testing.T) {
	b := NewAckBoard()
	b.Raise(2, 1, 5)
	b.Raise(2, 1, 3)
	if got := b.Acked(2, 1); got != 5 {
		t.Fatalf("the board holds %d, want 5", got)
	}
	if b.Acked(1, 2) != 0 {
		t.Fatal("the board holds a number for another pair")
	}
}

// aLogThatSaysWhatIsSafe is a replay log for the tests of acks: what it says is safe is set by the
// test.
type aLogThatSaysWhatIsSafe struct {
	replay.MemLog
	safe map[sim.StateID]uint64
}

func (l *aLogThatSaysWhatIsSafe) Safe(engine sim.StateID) uint64 { return l.safe[engine] }

func twoToysThatAckWhatIsSafe(t *testing.T) (*Cluster, *Engine[*toyState], *Engine[*toyState],
	*aLogThatSaysWhatIsSafe) {
	t.Helper()
	env := newToyEnv(t)
	log := &aLogThatSaysWhatIsSafe{safe: map[sim.StateID]uint64{}}
	env.ReplayLog = log
	c := NewCluster(env)
	a, b := New(&toyState{id: 1}, env, 1), New(&toyState{id: 2}, env, 1)
	c.Add(a)
	c.Add(b)
	return c, a, b, log
}

func TestAMessageIsHeldAndSentAgainUntilItsReceiverSaysItIsSafeAndThenLetGoOf(t *testing.T) {
	c, a, b, log := twoToysThatAckWhatIsSafe(t)
	held := &a.peer.data.unsettledMessages
	first := uint64(unsettledResendFirst.Get(config.Defaults()))
	a.ScheduleAt(1, &forwardAction{To: 2, N: 7})
	c.Step()
	if b.State().Total != 7 || held.Len() != 1 {
		t.Fatalf("b has %d and a holds %d, want 7 and 1",
			b.State().Total, held.Len())
	}
	sent := held.Sent()[0].Head()
	if sent.From != 1 || sent.Tick != 1 {
		t.Fatalf("the message went with the header %+v", sent)
	}
	for a.Tick() < first+2 { // b has not said it is safe: a sends the message again
		c.Step()
	}
	if b.State().Total != 7 {
		t.Fatalf("b took a second copy: it has %d", b.State().Total)
	}
	if held.Len() != 1 || held.Sent()[0].Head() != sent {
		t.Fatalf("a holds %d messages, and the first has the header %+v, want it unchanged",
			held.Len(), held.Sent()[0].Head())
	}
	log.safe[2] = b.Tick() // the tick at which b took it is safe
	c.Step()               // b says it
	c.Step()               // a reads it
	if held.Len() != 0 {
		t.Fatalf("a holds %d messages after the ack, want none", held.Len())
	}
}

func TestAnAckOnlyCoversTheTicksThatAreSafe(t *testing.T) {
	c, a, b, log := twoToysThatAckWhatIsSafe(t)
	held := &a.peer.data.unsettledMessages
	for _, tick := range []uint64{1, 4, 7} {
		a.ScheduleAt(tick, &forwardAction{To: 2, N: 1})
	}
	for range 8 {
		c.Step()
	}
	if held.Len() != 3 || b.State().Total != 3 {
		t.Fatalf("a holds %d and b has %d, want 3 and 3",
			held.Len(), b.State().Total)
	}
	log.safe[2] = 5 // b took the messages at the ticks 1 and 4, which are safe, not the one at 7
	c.Step()
	c.Step()
	if held.Len() != 1 || held.Sent()[0].Head().Tick != 7 {
		t.Fatalf("a holds %d messages: want only the one that b took at tick 7",
			held.Len())
	}
}

func TestAMessageWhoseReceiverIsGoneComesBackOnceAndIsLetGoOf(t *testing.T) {
	c, a, b, _ := twoToysThatAckWhatIsSafe(t)
	held := &a.peer.data.unsettledMessages
	first := uint64(unsettledResendFirst.Get(config.Defaults()))
	a.ScheduleAt(1, &forwardAction{To: 2, N: 7})
	c.Step()
	c.Env.Router.Retire(b.StateID())
	for a.Tick() < first+3 {
		c.Step()
	}
	if held.Len() != 0 {
		t.Fatalf("a still holds %d messages for a receiver that is gone", held.Len())
	}
	returned := 0
	for _, r := range c.Env.ReplayLog.(*aLogThatSaysWhatIsSafe).Records {
		if r.Kind() == replay.KindReturned && r.Engine == 1 {
			returned++
		}
	}
	if returned != 1 {
		t.Fatalf("%d messages came back, want 1", returned)
	}
}

func TestARestoredEngineSendsAgainWhatItHeldAndAcksWhatItTookAtOnce(t *testing.T) {
	c, a, b, _ := twoToysThatAckWhatIsSafe(t)
	a.ScheduleAt(1, &forwardAction{To: 2, N: 7})
	c.Step()
	data, err := b.Checkpoint().Encode()
	if err != nil {
		t.Fatal(err)
	}
	env := newToyEnv(t)
	restored, err := restoreEngine[*toyState](data, env, 1)
	if err != nil {
		t.Fatal(err)
	}
	// the state of b was restored from the checkpoint: what it took is in a place that no crash
	// takes back, so a sender is told of it without waiting
	if got := env.Acks.Acked(2, 1); got != a.peer.data.unsettledMessages.Sent()[0].Head().Seq {
		t.Fatalf("a restored b acks %d to a, want the number of the message it took", got)
	}
	if restored.State().Total != 7 {
		t.Fatalf("the restored b has %d, want 7", restored.State().Total)
	}
}

func TestAnEngineThatIsRemovedEndsItsLogWithAFrameThatSaysSo(t *testing.T) {
	dir := t.TempDir()
	env := newToyEnv(t)
	log := replay.NewFileLog(dir, replay.FileLogOptions{
		SegmentBytes: 1 << 20, SyncEvery: time.Hour, Backlog: 4,
	})
	env.ReplayLog = log
	c := NewCluster(env)
	first := New(&toyState{id: 1}, env, 1)
	c.Add(first)
	for range 3 {
		first.Send(toyCommand{N: 1})
		c.Step()
	}
	if !c.Remove(1) || len(c.Engines()) != 0 {
		t.Fatal("the engine was not removed")
	}
	reason, ok := env.Router.Deliver(toMessage(1, 1))
	if ok || reason != sim.DeliveryFailureRetired {
		t.Fatalf("a message to the engine that was removed bounced as %v, %v", reason, ok)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	codecs, err := toyCodecs()
	if err != nil {
		t.Fatal(err)
	}
	r, err := replay.ReadFileLog(dir, 1, 0, codecs)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var ticks []uint64
	var retiredAt uint64
	for r.Next() {
		rec := r.Record()
		if rec.Kind() == replay.KindRetired {
			retiredAt = rec.Tick
		}
		if rec.Kind() == replay.KindCommand || rec.Kind() == replay.KindRetired {
			ticks = append(ticks, rec.Tick)
		}
	}
	if r.Err() != nil {
		t.Fatal(r.Err())
	}
	if want := []uint64{1, 2, 3, 4}; !reflect.DeepEqual(ticks, want) || retiredAt != 4 {
		t.Fatalf("the ticks are %v, the removal at %d; want %v and 4", ticks, retiredAt, want)
	}
}

func TestManyGoroutinesRaiseTheBoardAndItEndsAtTheGreatestNumber(t *testing.T) {
	b := NewAckBoard()
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for n := range uint64(1000) {
				b.Raise(2, 1, n*8+uint64(g))
				b.Acked(2, 1)
			}
		})
	}
	wg.Wait()
	if got := b.Acked(2, 1); got != 999*8+7 {
		t.Fatalf("the board holds %d, want %d", got, 999*8+7)
	}
}

// BenchmarkAckBoard is what every engine pays each tick to say what it holds and to read what its
// receivers hold, on a board that all of them share.
func BenchmarkAckBoard(b *testing.B) {
	board := NewAckBoard()
	b.RunParallel(func(pb *testing.PB) {
		var seq uint64
		for pb.Next() {
			seq++
			board.Raise(2, 1, seq)
			board.Acked(2, 1)
		}
	})
}
