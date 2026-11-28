package engine_test

import (
	"bytes"
	"maps"
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/runtime/cluster"
	"github.com/huykingsofm/project-b/platform/runtime/cluster/stepping"
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/runtime/testkit"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/shared/testing/aliasing"
)

// offeredCheckpoint is a checkpoint that an engine handed to a recordingWriter.
type offeredCheckpoint struct {
	engine sim.StateID
	tick   uint64
	encode func() ([]byte, error)
}

// recordingWriter keeps the checkpoints it is offered, and writes none.
type recordingWriter struct{ offered []offeredCheckpoint }

func (w *recordingWriter) Offer(engine sim.StateID, tick uint64, encode func() ([]byte, error)) {
	w.offered = append(w.offered, offeredCheckpoint{engine: engine, tick: tick, encode: encode})
}

// toyClusterThatCheckpoints is a cluster of toy engines that take a checkpoint every 5 ticks and
// hash every 1,000, with the writer given, which may be nil.
func toyClusterThatCheckpoints(
	t *testing.T, w engine.CheckpointWriter, n int,
) (*cluster.Cluster, []*engine.Engine[*testkit.ToyState]) {
	t.Helper()
	env := testkit.NewToyEnv(t)
	env.CheckpointWriter = w
	c := testkit.NewCluster(t, env)
	testkit.SetConfig(t, c, map[string]string{
		"checkpoint.every_ticks": "5", "eventlog.hash_every_ticks": "1000",
	})
	return c, testkit.AddToys(t, c, n)
}

// hashTicks returns, for each engine, the ticks at which its log has a hash.
func hashTicks(mem *eventlog.MemLog) map[sim.StateID][]uint64 {
	out := map[sim.StateID][]uint64{}
	for _, r := range mem.Records {
		if r.Kind() == eventlog.KindHash {
			out[r.Engine] = append(out[r.Engine], r.Tick)
		}
	}
	return out
}

func TestAnEngineTakesACheckpointEveryPeriodFromItsOwnOffsetAndHashesAtIt(t *testing.T) {
	const engineCount = 4
	w := &recordingWriter{}
	c, engines := toyClusterThatCheckpoints(t, w, engineCount)
	for range 30 {
		if err := stepping.Step(c); err != nil {
			t.Fatal(err)
		}
	}
	offered := map[sim.StateID][]uint64{}
	for _, o := range w.offered {
		offered[o.engine] = append(offered[o.engine], o.tick)
	}
	hashed := hashTicks(c.Env.EventLog.(*eventlog.MemLog))
	starts := map[uint64]bool{}
	for _, e := range engines {
		id := e.StateID()
		ticks := offered[id]
		if len(ticks) != 6 || ticks[0] < 1 || ticks[0] > 5 {
			t.Fatalf("%v took checkpoints at %v, want 6, the first within the first 5 ticks",
				id, ticks)
		}
		for i := 1; i < len(ticks); i++ {
			if ticks[i]-ticks[i-1] != 5 {
				t.Fatalf("%v took checkpoints at %v, want one every 5 ticks", id, ticks)
			}
		}
		for _, tick := range ticks {
			if !slices.Contains(hashed[id], tick) {
				t.Fatalf("%v hashed at %v, want a hash at every checkpoint: %v", id, hashed[id],
					ticks)
			}
		}
		starts[ticks[0]] = true
	}
	if len(starts) < 2 {
		t.Fatalf("every engine takes its checkpoints at the same ticks: %v", offered)
	}

	// an engine with no writer takes none, and hashes at the same ticks, as a replay must
	plain, _ := toyClusterThatCheckpoints(t, nil, engineCount)
	for range 30 {
		if err := stepping.Step(plain); err != nil {
			t.Fatal(err)
		}
	}
	if got := hashTicks(plain.Env.EventLog.(*eventlog.MemLog)); !maps.EqualFunc(
		got, hashed, slices.Equal,
	) {
		t.Fatalf("without a writer the hashes are at %v, with one at %v", got, hashed)
	}
}

func TestACheckpointTakenAtATickIsOfThatTickHoweverFarTheEngineWentOnBeforeItIsEncoded(
	t *testing.T,
) {
	w := &recordingWriter{}
	c, engines := toyClusterThatCheckpoints(t, w, 1)
	e := engines[0]
	e.ScheduleAt(100, testkit.ToyAction{N: 1000}) // waits in the scheduler
	var want []byte
	var hashAt uint64
	for len(w.offered) == 0 {
		e.SendCommand(testkit.ToyCommand{N: 1})
		if err := stepping.Step(c); err != nil {
			t.Fatal(err)
		}
		var err error
		if want, err = e.Checkpoint().Encode(); err != nil {
			t.Fatal(err)
		}
		hashAt = e.Hash(sim.NewHasher())
	}
	taken := w.offered[0]
	for range 12 { // the state, the scheduler and the random numbers all move on
		e.SendCommand(testkit.ToyCommand{N: 7})
		e.ScheduleAt(e.Tick()+50, testkit.ToyAction{N: 3})
		if err := stepping.Step(c); err != nil {
			t.Fatal(err)
		}
	}
	got, err := taken.encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("the checkpoint is not the one of the tick it was taken at")
	}
	restored, err := restoreEngine[*testkit.ToyState](got, testkit.NewToyEnv(t))
	if err != nil {
		t.Fatal(err)
	}
	if restored.Tick() != taken.tick || restored.Hash(sim.NewHasher()) != hashAt {
		t.Fatalf("the restored engine is at tick %d with another hash, want tick %d",
			restored.Tick(), taken.tick)
	}
	var logged uint64
	for _, r := range c.Env.EventLog.(*eventlog.MemLog).Records {
		if r.Kind() == eventlog.KindHash && r.Tick == taken.tick {
			logged = r.Body.(eventlog.Hash).Value
		}
	}
	if logged != hashAt {
		t.Fatalf("the log has the hash %d at the tick of the checkpoint, want %d", logged, hashAt)
	}
}

// restoreEngine reads a checkpoint and makes its engine, in the environment given.
func restoreEngine[S sim.State](data []byte, env *engine.Environment) (*engine.Engine[S], error) {
	c, err := engine.DecodeCheckpoint[S](data, env.Codecs)
	if err != nil {
		return nil, err
	}
	return c.Restore(env)
}

func TestACheckpointSharesNoMemoryWithTheEngineItWasTakenFrom(t *testing.T) {
	c, engines := testkit.NewToyCluster(t, 2)
	e := engines[0]
	e.ScheduleAt(40, testkit.ToyAction{N: 1})
	e.ScheduleAt(40, testkit.ForwardAction{To: 2, N: 3})
	e.ScheduleAt(1, compensatedForwardAction{To: 2, Back: 1, N: 2})
	if err := stepping.Step(c); err != nil {
		t.Fatal(err)
	}
	taken := e.Checkpoint()
	for name, pair := range map[string][2]any{
		"the state":                  {e.State(), taken.State},
		"the scheduler":              {e.Scheduler(), &taken.ActionScheduler},
		"the state and the peer":     {e.State(), &taken.Peer},
		"the scheduler and the peer": {e.Scheduler(), &taken.Peer},
	} {
		if err := aliasing.Disjoint(pair[0], pair[1]); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

// otherToy is a state of another type.
type otherToy struct{ testkit.ToyState }

func (*otherToy) Type() sim.StateType { return testkit.ToyType + 1 }

func (s *otherToy) Clone() sim.State { c := *s; return &c }

func TestRestoringACheckpointOfAnotherTypeOfStateOrCutIsAnErrorAndRegistersNothing(t *testing.T) {
	c, engines := testkit.NewToyCluster(t, 1)
	engines[0].ScheduleAt(30, testkit.ForwardAction{To: 1, N: 2, Again: 10})
	stepping.Step(c)
	data, err := engines[0].Checkpoint().Encode()
	if err != nil {
		t.Fatal(err)
	}
	env := testkit.NewToyEnv(t)
	if _, err := restoreEngine[*testkit.ToyState](data, env); err != nil {
		t.Fatal(err)
	}
	for n := range data { // every proper prefix
		fresh := testkit.NewToyEnv(t)
		if _, err := restoreEngine[*testkit.ToyState](data[:n], fresh); err == nil {
			t.Fatalf("%d of %d bytes were restored", n, len(data))
		}
		m := sim.NewActionMessage[*testkit.ToyState](1, onfail.Drop{}, testkit.ToyAction{})
		if reason, ok := fresh.Router.Deliver(m); ok || reason != sim.DeliveryFailureNotStarted {
			t.Fatalf("a checkpoint that was cut at %d registered an engine in the router", n)
		}
	}
	if _, err := restoreEngine[*otherToy](data, testkit.NewToyEnv(t)); err == nil {
		t.Fatal("a checkpoint was restored as an engine of another type of state")
	}
}

func TestARestoredEngineHasTheHashOfTheOneItCameFromAndGoesOnTheSame(t *testing.T) {
	c, engines := testkit.NewToyCluster(t, 2)
	a := engines[0]
	a.ScheduleAt(3, testkit.ForwardAction{To: 2, N: 4, Again: 5})
	a.SendCommand(testkit.ToyCommand{N: 3})
	for range 8 {
		stepping.Step(c)
	}
	data, err := a.Checkpoint().Encode()
	if err != nil {
		t.Fatal(err)
	}
	// the restored engine runs in a cluster of its own, with an engine 2 of its own
	env := testkit.NewToyEnv(t)
	r, err := restoreEngine[*testkit.ToyState](data, env)
	if err != nil {
		t.Fatal(err)
	}
	if hashOf(r) != hashOf(a) || r.Tick() != a.Tick() || r.State().Total != a.State().Total {
		t.Fatalf("hash %d tick %d, want %d and %d", hashOf(r), r.Tick(), hashOf(a), a.Tick())
	}
	again, err := r.Checkpoint().Encode()
	if err != nil || string(again) != string(data) {
		t.Fatalf("a restored engine writes another checkpoint: %v", err)
	}
	if pending, _ := r.Scheduler().FindByID(testkit.ForwardActionID); pending == nil {
		t.Fatal("what the engine waited for is gone")
	}
}

func TestAnEngineHoldsWhatItSentAndWhatItTookFromEachSenderThroughACheckpoint(t *testing.T) {
	c, engines := testkit.NewToyCluster(t, 2)
	e := engines[0]
	e.ScheduleAt(1, testkit.ForwardAction{To: 2, N: 1})
	from99 := sim.NewActionMessage[*testkit.ToyState](1, onfail.Drop{}, testkit.ToyAction{N: 5}).
		Stamp(sim.Header{From: 99, Seq: 1})
	if _, ok := c.Env.Router.Deliver(from99); !ok {
		t.Fatal("the message of the sender 99 was refused")
	}
	if err := stepping.Step(c); err != nil {
		t.Fatal(err)
	}
	if len(e.GetSentMessages()) != 1 || e.State().Total != 5 {
		t.Fatalf("the engine holds %d sent messages and has %d, want 1 and 5",
			len(e.GetSentMessages()), e.State().Total)
	}
	data, err := e.Checkpoint().Encode()
	if err != nil {
		t.Fatal(err)
	}
	env := testkit.NewToyEnv(t)
	r, err := restoreEngine[*testkit.ToyState](data, env)
	if err != nil {
		t.Fatal(err)
	}
	got, want := r.GetSentMessages(), e.GetSentMessages()
	if len(got) != 1 || got[0].Head() != want[0].Head() || hashOf(r) != hashOf(e) {
		t.Fatalf("the engine made again holds %v, want %v, and the hash of the first", got, want)
	}
	// it remembers what it took from each sender: a second copy of the message is not taken
	if _, ok := env.Router.Deliver(from99); !ok {
		t.Fatal("the copy of the message of the sender 99 was refused")
	}
	if err := r.Step(); err != nil {
		t.Fatal(err)
	}
	if r.State().Total != 5 {
		t.Fatalf("the engine made again has %d, want 5: a copy is not taken twice", r.State().Total)
	}
}
