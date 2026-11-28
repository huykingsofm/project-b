package engine_test

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/runtime/cluster"
	"github.com/huykingsofm/project-b/platform/runtime/cluster/stepping"
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/runtime/stable"
	"github.com/huykingsofm/project-b/platform/runtime/testkit"
	"github.com/huykingsofm/project-b/platform/sim"
)

// resendFirstTicks is the default of unsettled_messages.resend_first_ticks: how many ticks a
// message that went waits for its ack before it is sent again the first time.
const resendFirstTicks = 20

func TestTheBoardOnlyGoesUp(t *testing.T) {
	b := stable.NewAckBoard()
	b.Raise(2, 1, 5)
	b.Raise(2, 1, 3)
	if got := b.Acked(2, 1); got != 5 {
		t.Fatalf("the board holds %d, want 5", got)
	}
	if b.Acked(1, 2) != 0 {
		t.Fatal("the board holds a number for another pair")
	}
}

// aLogThatSaysWhatIsSafe is a event log for the tests of acks: what it says is safe is set by the
// test.
type aLogThatSaysWhatIsSafe struct {
	eventlog.MemLog
	safe map[sim.StateID]uint64
}

func (l *aLogThatSaysWhatIsSafe) Safe(engine sim.StateID) uint64 { return l.safe[engine] }

func twoToysThatAckWhatIsSafe(t *testing.T) (*cluster.Cluster, *engine.Engine[*testkit.ToyState],
	*engine.Engine[*testkit.ToyState],
	*aLogThatSaysWhatIsSafe) {
	t.Helper()
	env := testkit.NewToyEnv(t)
	log := &aLogThatSaysWhatIsSafe{safe: map[sim.StateID]uint64{}}
	env.EventLog = log
	c := testkit.NewCluster(t, env)
	engines := testkit.AddToys(t, c, 2)
	return c, engines[0], engines[1], log
}

func TestAMessageIsHeldAndSentAgainUntilItsReceiverSaysItIsSafeAndThenLetGoOf(t *testing.T) {
	c, a, b, log := twoToysThatAckWhatIsSafe(t)
	first := uint64(resendFirstTicks)
	a.ScheduleAt(1, testkit.ForwardAction{To: 2, N: 7})
	stepping.Step(c)
	if b.State().Total != 7 || len(a.GetSentMessages()) != 1 {
		t.Fatalf("b has %d and a holds %d, want 7 and 1",
			b.State().Total, len(a.GetSentMessages()))
	}
	sent := a.GetSentMessages()[0].Head()
	if sent.From != 1 || sent.Tick != 1 {
		t.Fatalf("the message went with the header %+v", sent)
	}
	for a.Tick() < first+2 { // b has not said it is safe: a sends the message again
		stepping.Step(c)
	}
	if b.State().Total != 7 {
		t.Fatalf("b took a second copy: it has %d", b.State().Total)
	}
	if len(a.GetSentMessages()) != 1 || a.GetSentMessages()[0].Head() != sent {
		t.Fatalf("a holds %d messages, and the first has the header %+v, want it unchanged",
			len(a.GetSentMessages()), a.GetSentMessages()[0].Head())
	}
	log.safe[2] = b.Tick() // the tick at which b took it is safe
	stepping.Step(c)       // b says it
	stepping.Step(c)       // a reads it
	if len(a.GetSentMessages()) != 0 {
		t.Fatalf("a holds %d messages after the ack, want none", len(a.GetSentMessages()))
	}
}

func TestAnAckOnlyCoversTheTicksThatAreSafe(t *testing.T) {
	c, a, b, log := twoToysThatAckWhatIsSafe(t)
	for _, tick := range []uint64{1, 4, 7} {
		a.ScheduleAt(tick, testkit.ForwardAction{To: 2, N: 1})
	}
	for range 8 {
		stepping.Step(c)
	}
	if len(a.GetSentMessages()) != 3 || b.State().Total != 3 {
		t.Fatalf("a holds %d and b has %d, want 3 and 3",
			len(a.GetSentMessages()), b.State().Total)
	}
	log.safe[2] = 5 // b took the messages at the ticks 1 and 4, which are safe, not the one at 7
	stepping.Step(c)
	stepping.Step(c)
	if len(a.GetSentMessages()) != 1 || a.GetSentMessages()[0].Head().Tick != 7 {
		t.Fatalf("a holds %d messages: want only the one that b took at tick 7",
			len(a.GetSentMessages()))
	}
}

func TestAMessageWhoseReceiverIsGoneComesBackOnceAndIsLetGoOf(t *testing.T) {
	c, a, b, _ := twoToysThatAckWhatIsSafe(t)
	first := uint64(resendFirstTicks)
	a.ScheduleAt(1, testkit.ForwardAction{To: 2, N: 7})
	stepping.Step(c)
	c.Env.Router.MarkRetired(b.StateID())
	for a.Tick() < first+3 {
		stepping.Step(c)
	}
	if len(a.GetSentMessages()) != 0 {
		t.Fatalf("a still holds %d messages for a receiver that is gone", len(a.GetSentMessages()))
	}
	returned := 0
	for _, r := range c.Env.EventLog.(*aLogThatSaysWhatIsSafe).Records {
		if r.Kind() == eventlog.KindReturned && r.Engine == 1 {
			returned++
		}
	}
	if returned != 1 {
		t.Fatalf("%d messages came back, want 1", returned)
	}
}

func TestAMessageThatCameBackFromAReceiverRetiredWhileItWaitedIsLetGoOfAtOnceAndComesBackOnce(
	t *testing.T,
) {
	c, a, b, _ := twoToysThatAckWhatIsSafe(t)
	first := uint64(resendFirstTicks)
	a.ScheduleAt(1, testkit.ForwardAction{To: 2, N: 7})
	if err := a.Step(); err != nil { // a sends the message, which b has not taken yet
		t.Fatal(err)
	}
	if len(a.GetSentMessages()) != 1 {
		t.Fatalf("a holds %d messages after it sent one, want 1", len(a.GetSentMessages()))
	}
	b.Retire() // what waits in the channel of b goes back to a
	if err := a.Step(); err != nil {
		t.Fatal(err)
	}
	if len(a.GetSentMessages()) != 0 {
		t.Fatalf("a holds %d messages after the one it sent came back, want none",
			len(a.GetSentMessages()))
	}
	for a.Tick() < first+3 { // the tick at which it would be sent again goes by
		if err := a.Step(); err != nil {
			t.Fatal(err)
		}
	}
	returned := 0
	for _, r := range c.Env.EventLog.(*aLogThatSaysWhatIsSafe).Records {
		if r.Kind() == eventlog.KindReturned && r.Engine == 1 {
			returned++
		}
	}
	if returned != 1 {
		t.Fatalf("%d messages came back to a, want 1", returned)
	}
}

func TestARestoredEngineSendsAgainWhatItHeldAndAcksWhatItTookAtOnce(t *testing.T) {
	c, a, b, _ := twoToysThatAckWhatIsSafe(t)
	a.ScheduleAt(1, testkit.ForwardAction{To: 2, N: 7})
	stepping.Step(c)
	data, err := b.Checkpoint().Encode()
	if err != nil {
		t.Fatal(err)
	}
	env := testkit.NewToyEnv(t)
	restored, err := restoreEngine[*testkit.ToyState](data, env)
	if err != nil {
		t.Fatal(err)
	}
	// the state of b was restored from the checkpoint: what it took is in a place that no crash
	// takes back, so a sender is told of it without waiting
	if got := env.AckBoard.Acked(2, 1); got != a.GetSentMessages()[0].Head().Seq {
		t.Fatalf("a restored b acks %d to a, want the number of the message it took", got)
	}
	if restored.State().Total != 7 {
		t.Fatalf("the restored b has %d, want 7", restored.State().Total)
	}
}

func TestAnEngineThatIsRetiredEndsItsLogWithAFrameThatSaysSo(t *testing.T) {
	dir := t.TempDir()
	env := testkit.NewToyEnv(t)
	log := eventlog.NewFileLog(dir, eventlog.FileLogOptions{
		SegmentBytes: 1 << 20, SyncEvery: time.Hour, Backlog: 4,
	})
	env.EventLog = log
	c := testkit.NewCluster(t, env)
	first := testkit.AddToys(t, c, 1)[0]
	for range 3 {
		first.SendCommand(testkit.ToyCommand{N: 1})
		stepping.Step(c)
	}
	// the coordinator engine stays
	if !stepping.RetireEngine(c, 1) || len(c.Engines()) != 1 {
		t.Fatal("the engine was not retired")
	}
	reason, ok := env.Router.Deliver(testkit.ToyMessage(1, 1))
	if ok || reason != sim.DeliveryFailureRetired {
		t.Fatalf("a message to the engine that was retired bounced as %v, %v", reason, ok)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	codecs, _, err := testkit.ToyCodecs()
	if err != nil {
		t.Fatal(err)
	}
	r, err := eventlog.ReadFileLog(dir, 1, 0, codecs)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var ticks []uint64
	var retiredAt uint64
	for r.Next() {
		rec := r.Record()
		if rec.Kind() == eventlog.KindRetired {
			retiredAt = rec.Tick
		}
		if rec.Kind() == eventlog.KindCommand || rec.Kind() == eventlog.KindRetired {
			ticks = append(ticks, rec.Tick)
		}
	}
	if r.Err() != nil {
		t.Fatal(r.Err())
	}
	if want := []uint64{1, 2, 3, 4}; !reflect.DeepEqual(ticks, want) || retiredAt != 4 {
		t.Fatalf("the ticks are %v, the retirement at %d; want %v and 4", ticks, retiredAt, want)
	}
}

func TestManyGoroutinesRaiseTheBoardAndItEndsAtTheGreatestNumber(t *testing.T) {
	b := stable.NewAckBoard()
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
	board := stable.NewAckBoard()
	b.RunParallel(func(pb *testing.PB) {
		var seq uint64
		for pb.Next() {
			seq++
			board.Raise(2, 1, seq)
			board.Acked(2, 1)
		}
	})
}
