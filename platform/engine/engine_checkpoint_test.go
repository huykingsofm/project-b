package engine

import (
	"bytes"
	"maps"
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/eventlog"
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
	t *testing.T, w CheckpointWriter, ids ...sim.StateID,
) (*Cluster, []*Engine[*toyState]) {
	t.Helper()
	env := newToyEnv(t)
	env.Checkpoints = w
	if _, err := env.Cfg.(*config.Store).Apply(map[string]string{
		"checkpoint.every_ticks": "5", "eventlog.hash_every_ticks": "1000",
	}); err != nil {
		t.Fatal(err)
	}
	c := NewCluster(env)
	var engines []*Engine[*toyState]
	for _, id := range ids {
		e := New(&toyState{id: id}, env, 1)
		engines = append(engines, e)
		c.Add(e)
	}
	return c, engines
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
	ids := []sim.StateID{1, 2, 3, 4}
	w := &recordingWriter{}
	c, _ := toyClusterThatCheckpoints(t, w, ids...)
	for range 30 {
		if err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	offered := map[sim.StateID][]uint64{}
	for _, o := range w.offered {
		offered[o.engine] = append(offered[o.engine], o.tick)
	}
	hashed := hashTicks(c.Env.EventLog.(*eventlog.MemLog))
	starts := map[uint64]bool{}
	for _, id := range ids {
		var want []uint64
		for tick := uint64(1); tick <= 30; tick++ {
			if (tick+periodOffsetOf(id)%5)%5 == 0 {
				want = append(want, tick)
			}
		}
		if !slices.Equal(offered[id], want) {
			t.Fatalf("%v took checkpoints at %v, want %v", id, offered[id], want)
		}
		var wantHashes []uint64
		for tick := uint64(1); tick <= 30; tick++ {
			if slices.Contains(want, tick) || (tick+periodOffsetOf(id)%1000)%1000 == 0 {
				wantHashes = append(wantHashes, tick)
			}
		}
		if !slices.Equal(hashed[id], wantHashes) {
			t.Fatalf("%v hashed at %v, want %v: every checkpoint has its hash", id, hashed[id],
				wantHashes)
		}
		starts[want[0]] = true
	}
	if len(starts) < 2 {
		t.Fatalf("every engine takes its checkpoints at the same ticks: %v", offered)
	}

	// an engine with no writer takes none, and hashes at the same ticks, as a replay must
	plain, _ := toyClusterThatCheckpoints(t, nil, ids...)
	for range 30 {
		if err := plain.Step(); err != nil {
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
	e.ScheduleAt(100, toyAction{N: 1000}) // waits in the scheduler
	var want []byte
	var hashAt uint64
	for len(w.offered) == 0 {
		e.Send(toyCommand{N: 1})
		if err := c.Step(); err != nil {
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
		e.Send(toyCommand{N: 7})
		e.ScheduleAt(e.Tick()+50, toyAction{N: 3})
		e.rng.Uint64()
		if err := c.Step(); err != nil {
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
	restored, err := restoreEngine[*toyState](got, newToyEnv(t), 1)
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
func restoreEngine[S sim.State](data []byte, env *Env, seed uint64) (*Engine[S], error) {
	c, err := DecodeEngineCheckpoint[S](data, env.Codecs)
	if err != nil {
		return nil, err
	}
	return c.Restore(env, seed)
}

func TestACheckpointSharesNoMemoryWithTheEngineItWasTakenFrom(t *testing.T) {
	c, engines := toyCluster(t, 1)
	e := engines[0]
	e.ScheduleAt(40, toyAction{N: 1})
	e.ScheduleAt(40, forwardAction{To: 2, N: 3})
	e.peer.data.lastRecvSeq.Set(7, 3)
	e.peer.data.unsettledMessages.AddUnsent(
		sim.NewActionMessage(2, onfail.Notify{Key: "k", Args: []string{"a"}}, toyAction{N: 2}),
	)
	e.peer.data.unsettledMessages.SetSent([]sim.Message{
		sim.NewActionMessage(3, onfail.Compensate{
			With: sim.NewActionMessage(1, onfail.Drop{}, toyAction{N: 4}),
		}, toyAction{N: 5}).Stamp(sim.Header{From: 1, Seq: 1}),
	})
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	taken := e.Checkpoint()
	for name, pair := range map[string][2]any{
		"the state":                  {e.st, taken.State},
		"the scheduler":              {&e.actionScheduler, &taken.ActionScheduler},
		"the random numbers":         {e.rng, taken.Rand},
		"what the peer knows":        {&e.peer.data, &taken.Peer},
		"the state and the peer":     {e.st, &taken.Peer},
		"the scheduler and the peer": {&e.actionScheduler, &taken.Peer},
	} {
		if err := aliasing.Disjoint(pair[0], pair[1]); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if e.rng.Uint64() != taken.Rand.Uint64() {
		t.Fatal("the random numbers of the checkpoint do not go on from where the engine was")
	}
}

// otherToy is a state of another type.
type otherToy struct{ toyState }

func (*otherToy) Type() sim.StateType { return toyType + 1 }

func TestRestoringACheckpointOfAnotherTypeOfStateOrCutIsAnErrorAndRegistersNothing(t *testing.T) {
	c, engines := toyCluster(t, 1)
	engines[0].ScheduleAt(30, forwardAction{To: 1, N: 2, Again: 10})
	c.Step()
	data, err := engines[0].Checkpoint().Encode()
	if err != nil {
		t.Fatal(err)
	}
	env := newToyEnv(t)
	if _, err := restoreEngine[*toyState](data, env, 1); err != nil {
		t.Fatal(err)
	}
	for n := range data { // every proper prefix
		fresh := newToyEnv(t)
		if _, err := restoreEngine[*toyState](data[:n], fresh, 1); err == nil {
			t.Fatalf("%d of %d bytes were restored", n, len(data))
		}
		if _, ok := fresh.Router.(*Router).data.Load().chans[1]; ok {
			t.Fatalf("a checkpoint that was cut at %d registered an engine in the router", n)
		}
	}
	if _, err := restoreEngine[*otherToy](data, newToyEnv(t), 1); err == nil {
		t.Fatal("a checkpoint was restored as an engine of another type of state")
	}
}

func TestARestoredEngineHasTheHashOfTheOneItCameFromAndGoesOnTheSame(t *testing.T) {
	c, engines := toyCluster(t, 1, 2)
	a := engines[0]
	a.ScheduleAt(3, forwardAction{To: 2, N: 4, Again: 5})
	a.Send(toyCommand{N: 3})
	for range 8 {
		c.Step()
	}
	data, err := a.Checkpoint().Encode()
	if err != nil {
		t.Fatal(err)
	}
	// the restored engine runs in a cluster of its own, with an engine 2 of its own
	env := newToyEnv(t)
	r, err := restoreEngine[*toyState](data, env, 1)
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
	if pending, _ := r.Scheduler().FindByID(forwardActionID); pending == nil {
		t.Fatal("what the engine waited for is gone")
	}
}

// lastFrom99 is the last number that the engine took from the sender 99.
func lastFrom99(e *Engine[*toyState]) uint64 {
	seq, _ := e.peer.data.lastRecvSeq.Get(99)
	return seq
}

func TestAnEngineHoldsWhatItWasToSendAgainAndWhatItTookFromEachSenderThroughACheckpoint(
	t *testing.T,
) {
	_, engines := toyCluster(t, 1)
	e := engines[0]
	e.peer.data.sentSeq = 41
	e.peer.data.lastRecvSeq.Set(99, 1)
	e.peer.data.unsettledMessages.SetUnsent([]sim.Message{
		sim.NewActionMessage(2, onfail.Retry{}, toyAction{N: 1}).
			Stamp(sim.Header{From: 1, Seq: 40}),
	})
	e.peer.data.unsettledMessages.SetSent([]sim.Message{
		sim.NewActionMessage(3, onfail.Retry{}, toyAction{N: 2}).
			Stamp(sim.Header{From: 1, Seq: 39, Tick: 6}),
	})
	data, err := e.Checkpoint().Encode()
	if err != nil {
		t.Fatal(err)
	}
	r, err := restoreEngine[*toyState](data, newToyEnv(t), 1)
	if err != nil {
		t.Fatal(err)
	}
	held := &r.peer.data.unsettledMessages
	unsent, sent := len(held.Unsent()), len(held.Sent())
	if r.peer.data.sentSeq != 41 || unsent != 1 || sent != 1 ||
		lastFrom99(r) != 1 || hashOf(r) != hashOf(e) {
		t.Fatalf("seq %d, %d to go, %d sent, last %v",
			r.peer.data.sentSeq, unsent, sent, lastFrom99(r))
	}
	if got := held.Sent()[0].Head(); got.Seq != 39 || got.Tick != 6 {
		t.Fatalf("the message that was sent came back as %+v, want its number and tick", got)
	}
}
