package engine

import (
	"errors"
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/rand"
)

func newToyEnv(t testing.TB) *Env {
	t.Helper()
	codecs, err := toyCodecs()
	if err != nil {
		t.Fatal(err)
	}
	return &Env{
		Router: NewRouter(), Cfg: config.NewStore(), ReplayLog: &replay.MemLog{},
		Stable: NewStableStateStore(), Codecs: codecs, QueueSize: 16, Emitter: NopEmitter{},
		Acks: NewAckBoard(),
	}
}

func toyCluster(t testing.TB, ids ...sim.StateID) (*Cluster, []*Engine[*toyState]) {
	t.Helper()
	c := NewCluster(newToyEnv(t))
	var engines []*Engine[*toyState]
	for _, id := range ids {
		e := New(&toyState{id: id}, c.Env, 1)
		engines = append(engines, e)
		c.Add(e)
	}
	return c, engines
}

func TestAStepRunsTheCommandsPublishesTheStateAndRecordsTheCommand(t *testing.T) {
	c, engines := toyCluster(t, 1)
	a := engines[0]
	if !a.Send(toyCommand{N: 5}) {
		t.Fatal("the command was refused")
	}
	c.Step()
	if a.State().Total != 5 || a.Tick() != 1 {
		t.Fatalf("total %d at tick %d, want 5 at 1", a.State().Total, a.Tick())
	}
	published, err := c.Env.Stable.View(2, replay.NopLog{}).Read(1)
	if err != nil || published.(*toyState).Total != 5 || published.(*toyState) == a.State() {
		t.Fatalf("the published state is %+v, %v: want a copy with the total", published, err)
	}
	mem := c.Env.ReplayLog.(*replay.MemLog)
	commands := 0
	for _, r := range mem.Records {
		if r.Kind() == replay.KindCommand && r.Engine == 1 && r.Tick == 1 {
			commands++
		}
	}
	if commands != 1 {
		t.Fatalf("%d commands recorded, want 1", commands)
	}
}

func TestAMessageToAnotherEngineIsTakenAtItsNextTickAndACopyIsNeverTakenTwice(t *testing.T) {
	c, engines := toyCluster(t, 1, 2)
	a, b := engines[0], engines[1]
	a.ScheduleAt(1, &forwardAction{To: 2, N: 7})
	c.Step() // a sends: b runs after a in this step, so it takes the message at once
	if b.State().Total != 7 {
		t.Fatalf("b has %d, want 7", b.State().Total)
	}
	dup := sim.NewActionMessage(2, onfail.Drop{}, &toyAction{N: 100}).
		Stamp(sim.Header{From: 1, Seq: 1}) // the first message of a had the number 1
	c.Env.Router.Deliver(dup)
	c.Step()
	if b.State().Total != 7 {
		t.Fatalf("a second copy of a message was taken: %d", b.State().Total)
	}
}

func TestAMessageCarriesTheTickOfItsSenderAndTheReceiverDependsOnIt(t *testing.T) {
	c, engines := toyCluster(t, 1, 2)
	a := engines[0]
	a.ScheduleAt(3, &forwardAction{To: 2, N: 7})
	c.Step()
	c.Step()
	c.Step() // a sends at its tick 3: b takes the message in the same step
	mem := c.Env.ReplayLog.(*replay.MemLog)
	var deps []sim.StateDependency
	for _, r := range mem.Records {
		if dep, ok := r.Dependency(); ok && r.Engine == 2 {
			deps = append(deps, dep)
		}
	}
	if want := []sim.StateDependency{{Source: 1, Tick: 3}}; !reflect.DeepEqual(deps, want) {
		t.Fatalf("the dependencies of b are %v, want %v", deps, want)
	}
}

func TestAnEngineReadsWhatAnotherPublishedByItsID(t *testing.T) {
	c, engines := toyCluster(t, 1, 2)
	engines[0].Send(toyCommand{N: 9})
	c.Step()
	engines[1].ScheduleAt(2, &readAction{From: 1})
	c.Step()
	if engines[1].State().Total != 9 {
		t.Fatalf("b read %d of a, want 9", engines[1].State().Total)
	}
}

func TestAClusterFindsEnginesByIDAndTypeAndStepsThemInTheOrderTheyWereAdded(t *testing.T) {
	c, engines := toyCluster(t, 3, 1, 2)
	if got, ok := c.Get[*toyState](1); !ok || got != engines[1] {
		t.Fatal("Get did not find the engine with the ID 1")
	}
	if _, ok := c.Get[*toyState](9); ok {
		t.Fatal("Get found an engine that is not there")
	}
	if got := c.All[*toyState](); len(got) != 3 || got[0] != engines[0] || got[2] != engines[2] {
		t.Fatalf("All returned %v", got)
	}
	if stepper, ok := c.Find(2); !ok || stepper.Type() != toyType {
		t.Fatal("Find did not find the engine with the ID 2")
	}
}

func TestACommandOfAnotherTypeOfStateAndANilCommandAreRefused(t *testing.T) {
	_, engines := toyCluster(t, 1)
	e := engines[0]
	var none sim.Command[*toyState]
	if e.Send(none) {
		t.Fatal("a nil command was taken")
	}
	if err := e.SendBaseCommand(otherCommand{}); err == nil {
		t.Fatal("a command of another type of state was taken")
	}
	if err := e.SendBaseCommand(toyCommand{N: 1}); err != nil {
		t.Fatalf("a command of its own type: %v", err)
	}
	for range 20 {
		if err := e.SendBaseCommand(toyCommand{}); err != nil {
			return // the queue is full: an error, not a panic
		}
	}
	t.Fatal("the queue of 16 did not fill")
}

// otherCommand is a command that is not one of the toy state.
type otherCommand struct{}

func (otherCommand) ID() sim.CommandID           { return 60009 }
func (otherCommand) Encode(*binary.Writer) error { return nil }

// otherToy is a state of another type.
type otherToy struct{ toyState }

func (*otherToy) Type() sim.StateType { return toyType + 1 }

func TestRestoringACheckpointOfAnotherTypeOfStateOrCutIsAnErrorAndRegistersNothing(t *testing.T) {
	c, engines := toyCluster(t, 1)
	engines[0].ScheduleAt(30, &forwardAction{To: 1, N: 2, Again: 10})
	c.Step()
	data, err := CheckpointEngine(engines[0])
	if err != nil {
		t.Fatal(err)
	}
	env := newToyEnv(t)
	if _, err := RestoreEngine[*toyState](data, env, 1); err != nil {
		t.Fatal(err)
	}
	for n := range data { // every proper prefix
		fresh := newToyEnv(t)
		if _, err := RestoreEngine[*toyState](data[:n], fresh, 1); err == nil {
			t.Fatalf("%d of %d bytes were restored", n, len(data))
		}
		if _, ok := fresh.Router.(*Router).data.Load().chans[1]; ok {
			t.Fatalf("a checkpoint that was cut at %d registered an engine in the router", n)
		}
	}
	if _, err := RestoreEngine[*otherToy](data, newToyEnv(t), 1); err == nil {
		t.Fatal("a checkpoint was restored as an engine of another type of state")
	}
}

func TestARestoredEngineHasTheHashOfTheOneItCameFromAndGoesOnTheSame(t *testing.T) {
	c, engines := toyCluster(t, 1, 2)
	a := engines[0]
	a.ScheduleAt(3, &forwardAction{To: 2, N: 4, Again: 5})
	a.Send(toyCommand{N: 3})
	for range 8 {
		c.Step()
	}
	data, err := CheckpointEngine(a)
	if err != nil {
		t.Fatal(err)
	}
	// the restored engine runs in a cluster of its own, with an engine 2 of its own
	env := newToyEnv(t)
	r, err := RestoreEngine[*toyState](data, env, 1)
	if err != nil {
		t.Fatal(err)
	}
	if hashOf(r) != hashOf(a) || r.Tick() != a.Tick() || r.State().Total != a.State().Total {
		t.Fatalf("hash %d tick %d, want %d and %d", hashOf(r), r.Tick(), hashOf(a), a.Tick())
	}
	again, err := CheckpointEngine(r)
	if err != nil || string(again) != string(data) {
		t.Fatalf("a restored engine writes another checkpoint: %v", err)
	}
	if pending, _ := r.Scheduler().FindByID(forwardActionID); pending == nil {
		t.Fatal("what the engine waited for is gone")
	}
}

func TestTheHashOfAnEngineCoversWhatOnlyTheEngineKnows(t *testing.T) {
	_, engines := toyCluster(t, 1)
	e := engines[0]
	before := hashOf(e)
	e.cell.sentSeq++
	if hashOf(e) == before {
		t.Fatal("the number of its messages is not in the hash")
	}
	before = hashOf(e)
	e.cell.lastRecvSeq[7] = 3
	if hashOf(e) == before {
		t.Fatal("the last number taken from a sender is not in the hash")
	}
	before = hashOf(e)
	e.unsettledMessages.AddUnsent(sim.NewActionMessage[*toyState](1, onfail.Drop{}))
	if hashOf(e) == before {
		t.Fatal("a message held to be sent again is not in the hash")
	}
	before = hashOf(e)
	e.unsettledMessages.SetSent([]sim.Message{sim.NewActionMessage[*toyState](2, onfail.Drop{})})
	if hashOf(e) != before {
		t.Fatal("a message that was sent and is not acked is in the hash: when it is acked " +
			"depends on when the log of its receiver is synced, which a replay does not repeat")
	}
}

func TestAnEngineHoldsWhatItWasToSendAgainAndWhatItTookFromEachSenderThroughACheckpoint(
	t *testing.T,
) {
	_, engines := toyCluster(t, 1)
	e := engines[0]
	e.cell.sentSeq = 41
	e.cell.lastRecvSeq[99] = 1
	e.unsettledMessages.SetUnsent([]sim.Message{
		sim.NewActionMessage(2, onfail.Retry{}, &toyAction{N: 1}).
			Stamp(sim.Header{From: 1, Seq: 40}),
	})
	e.unsettledMessages.SetSent([]sim.Message{
		sim.NewActionMessage(3, onfail.Retry{}, &toyAction{N: 2}).
			Stamp(sim.Header{From: 1, Seq: 39, Tick: 6}),
	})
	data, err := CheckpointEngine(e)
	if err != nil {
		t.Fatal(err)
	}
	r, err := RestoreEngine[*toyState](data, newToyEnv(t), 1)
	if err != nil {
		t.Fatal(err)
	}
	unsent, sent := len(r.unsettledMessages.Unsent()), len(r.unsettledMessages.Sent())
	if r.cell.sentSeq != 41 || unsent != 1 || sent != 1 ||
		r.cell.lastRecvSeq[99] != 1 || hashOf(r) != hashOf(e) {
		t.Fatalf("seq %d, %d to go, %d sent, last %v",
			r.cell.sentSeq, unsent, sent, r.cell.lastRecvSeq)
	}
	if got := r.unsettledMessages.Sent()[0].Head(); got.Seq != 39 || got.Tick != 6 {
		t.Fatalf("the message that was sent came back as %+v, want its number and tick", got)
	}
}

// BenchmarkEngineHash is the hash of an engine that holds messages to send and messages that went
// and were not acked: what is taken at the period of the hash of the replay log.
func BenchmarkEngineHash(b *testing.B) {
	env := newToyEnv(b)
	e := New(&toyState{id: 1}, env, 1)
	cfg := config.Defaults()
	for i := range uint64(20) {
		e.cell.unsettledMessages.AddUnsent(toMessage(2, i+1))
		e.cell.unsettledMessages.MarkSent(toMessage(2, 100+i), 1, cfg)
	}
	b.ReportAllocs()
	for b.Loop() {
		hashOf(e)
	}
}

func TestTheHashOfACellThatCannotBeWrittenSaysSoAndStillTellsItFromAnother(t *testing.T) {
	good := cellData{rng: rand.NewPCG(1, 2), lastRecvSeq: map[sim.StateID]uint64{}}
	bad := good
	bad.unsettledMessages.AddUnsent(nil) // a message that has no codec
	a, b := sim.NewHasher(), sim.NewHasher()
	if err := good.Hash(&a); err != nil {
		t.Fatal(err)
	}
	if err := bad.Hash(&b); !errors.Is(err, sim.ErrNoCodec) {
		t.Fatalf("the hash of a cell that cannot be written: %v, want ErrNoCodec", err)
	}
	if a == b {
		t.Fatal("a cell that cannot be written has the hash of one that can")
	}
}

// hashOf is the hash of an engine, from a new hasher.
func hashOf[S sim.State](e *Engine[S]) uint64 { return e.Hash(sim.NewHasher()) }
