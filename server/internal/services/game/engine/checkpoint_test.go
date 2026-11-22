package engine

import (
	"bytes"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/onfail"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/command"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap/tilemaptest"
)

// aWorldToCheckpoint is a world of one zone with one zonelayer, that heals its characters every 10
// ticks.
func aWorldToCheckpoint(t testing.TB) *Cluster {
	t.Helper()
	tiles := tilemaptest.New(t, 10, 5, tilemap.Tile{X: 1, Y: 2}, nil, nil)
	c, err := NewBuilder().
		WithSeed(7).
		WithReplayLog(&replay.MemLog{}).
		WithWorld(WorldParams{ID: 1}).
		WithZone(ZoneParams{ID: 1, Tiles: tiles}).
		WithZoneLayers(ZoneLayerParams{ZoneID: 1, Count: 1, Capacity: 5, RegenEvery: 10}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func steps(c *Cluster, n int) {
	for range n {
		c.Step()
	}
}

// aWorldWithActionsThatWait has a character that runs and is shielded, and an item on the ground
// that will be taken off: so the zonelayer waits for things at a later tick and has set things for
// itself for the next one.
func aWorldWithActionsThatWait(t testing.TB) *Cluster {
	t.Helper()
	c := aWorldToCheckpoint(t)
	an, err := zonelayerstate.NewCharacter(1, "An", 100, 1000, 0)
	if err != nil {
		t.Fatal(err)
	}
	c.Zone(1).Send(command.ZoneCharacterEnterCommand{Character: an})
	steps(c, 4)
	c.Env.Router.Deliver(action.MessageToZoneLayer(
		1, onfail.Drop{},
		&action.ZoneLayerItemEntityAddAction{
			ItemID: 7, Count: 1, At: state.Vec{X: 2500, Y: 2500}, Expiry: 40,
		},
	).Stamp(message.Header{From: message.ZoneAddress(99), Seq: 1}))
	c.ZoneLayer(1).Send(command.ZoneLayerCharacterShieldGrantCommand{CharacterID: 1, Duration: 100})
	c.ZoneLayer(1).Send(
		command.ZoneLayerCharacterMoveCommand{CharacterID: 1, Dir: state.East, Ticks: 8},
	)
	steps(c, 3)
	c.Zone(1).Send(command.ZoneChatSendCommand{FromID: 1, Text: "hello"})
	c.World.Send(command.WorldChatPrivateSendCommand{FromID: 1, ToID: 2, Text: "psst"})
	steps(c, 2)
	return c
}

func TestARestoredZoneLayerHasTheHashOfTheOneItCameFromAndGoesOnTheSame(t *testing.T) {
	c := aWorldWithActionsThatWait(t)
	l := c.ZoneLayer(1)
	if l.sched.Pending() == 0 || len(l.selfNext) == 0 {
		t.Fatalf(
			"the zonelayer waits for %d actions and has set %d for itself: the test checks nothing",
			l.sched.Pending(), len(l.selfNext),
		)
	}
	data, err := l.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	r, err := RestoreZoneLayerEngine(data, c.Env, 0)
	if err != nil {
		t.Fatal(err)
	}

	if r.hash() != l.hash() || r.tick != l.tick || r.st.Hash() != l.st.Hash() {
		t.Fatal("the restored engine has another hash, or another tick")
	}
	if again, err := r.Checkpoint(); err != nil || !bytes.Equal(again, data) {
		t.Fatalf("the restored engine is written differently: %v", err)
	}
	if a, b := l.rng.Uint64(), r.rng.Uint64(); a != b {
		t.Fatalf("the random numbers go on with %d, and with %d in the original", b, a)
	}
	for tick := 1; tick <= 60; tick++ { // the item is taken off, the run ends, the heal comes twice
		l.Step()
		r.Step()
		if r.hash() != l.hash() {
			t.Fatalf("%d ticks after the restore the hash differs", tick)
		}
	}
	if l.sched.Pending() != 1 { // only the heal, which sets itself again, waits
		t.Fatalf("the original still waits for %d actions: the test did not run long enough",
			l.sched.Pending())
	}
}

func TestARestoredEngineKnowsWhatItTookFromEachSenderAndHoldsWhatItWasToSendAgain(t *testing.T) {
	c := aWorldWithActionsThatWait(t)
	l := c.ZoneLayer(1)
	undo := action.MessageToZone(1, onfail.Drop{}, &action.ZoneChatSendAction{FromID: 1, Text: "x"})
	l.bounce.pending = []message.Message{
		action.MessageToZoneLayer(
			1, onfail.Compensate{With: undo}, &action.ZoneLayerCharacterRegenAction{},
		).Stamp(message.Header{From: l.me, Seq: 9}).Again(),
	}
	l.seq = 41
	data, err := l.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	r, err := RestoreZoneLayerEngine(data, c.Env, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r.seq != 41 || len(r.bounce.pending) != 1 || r.lastSeq[message.ZoneAddress(99)] != 1 ||
		r.cfgVersionSeen != l.cfgVersionSeen {
		t.Fatalf("seq %d, %d held, last %v", r.seq, len(r.bounce.pending), r.lastSeq)
	}
	// a second copy of the message of the sender 99 is dropped by the restored engine, as it would
	// be by the original
	chat := action.MessageToZoneLayer(
		1, onfail.Drop{}, &action.ZoneLayerItemEntityAddAction{ItemID: 8, Count: 1},
	).Stamp(message.Header{From: message.ZoneAddress(99), Seq: 1})
	if _, ok := c.Env.Router.Deliver(chat); !ok {
		t.Fatal("the message was not delivered")
	}
	before := r.st.Entities.Len()
	r.Step()
	if r.st.Entities.Len() != before {
		t.Fatal("a second copy of a message that was taken before the checkpoint was taken again")
	}
}

func TestARestoredWorldAndZoneHaveTheHashOfTheOnesTheyCameFrom(t *testing.T) {
	c := aWorldWithActionsThatWait(t)
	world, err := c.World.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	w, err := RestoreWorldEngine(world, c.Env, 0)
	if err != nil || w.hash() != c.World.hash() || w.tick != c.World.tick {
		t.Fatalf("the world: %v", err)
	}
	if again, err := w.Checkpoint(); err != nil || !bytes.Equal(again, world) {
		t.Fatalf("the restored world is written differently: %v", err)
	}
	tiles := c.Zone(1).st.Tiles
	zone, err := c.Zone(1).Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	z, err := RestoreZoneEngine(zone, c.Env, 0, func(id state.ZoneID) *tilemap.TileMap {
		if id == 1 {
			return tiles
		}
		return nil
	})
	if err != nil || z.hash() != c.Zone(1).hash() || z.tick != c.Zone(1).tick {
		t.Fatalf("the zone: %v", err)
	}
	if again, err := z.Checkpoint(); err != nil || !bytes.Equal(again, zone) {
		t.Fatalf("the restored zone is written differently: %v", err)
	}
	if c.World.st.Private.Len() == 0 || c.Zone(1).st.Chat.Len() == 0 {
		t.Fatal("the world and the zone hold nothing to restore: the test checks nothing")
	}
}

func TestACheckpointThatIsCutOrOfAnotherScopeIsAnErrorAndNeverAPanic(t *testing.T) {
	c := aWorldWithActionsThatWait(t)
	data, err := c.ZoneLayer(1).Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	for n := range data { // every proper prefix
		if _, err := RestoreZoneLayerEngine(data[:n], c.Env, 0); err == nil {
			t.Fatalf("%d of %d bytes were restored", n, len(data))
		}
	}
	longer := append(append([]byte{}, data...), 0)
	if _, err := RestoreZoneLayerEngine(longer, c.Env, 0); err == nil {
		t.Fatal("a checkpoint with a byte more was restored")
	}
	if _, err := RestoreWorldEngine(data, c.Env, 0); err == nil {
		t.Fatal("the checkpoint of a zonelayer was restored as the world")
	}
	if _, err := RestoreZoneEngine(data, c.Env, 0, nil); err == nil {
		t.Fatal("the checkpoint of a zonelayer was restored as a zone")
	}
}

// aFreshEnv is an environment with a router, a registry and a configuration of its own, in which
// no engine has been made.
func aFreshEnv(c *Cluster) *Env {
	return &Env{
		Router: NewRouter(), Cfg: config.NewStore(), ReplayLog: replay.NopLog{},
		Stable: NewStableStateRegistry(1), Graph: c.Env.Graph, QueueSize: 16,
		WorldEmitter: NopEmitter{}, ZoneEmitter: NopEmitter{}, ZoneLayerEmitter: NopEmitter{},
	}
}

// A checkpoint that is bad makes no engine, and so registers none in the router: a message to it
// bounces as an engine that is not built, and does not wait in a queue that nothing reads.
func TestACheckpointThatIsBadRegistersNoEngineInTheRouter(t *testing.T) {
	c := aWorldWithActionsThatWait(t)
	data, err := c.ZoneLayer(1).Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	say := action.MessageToZoneLayer(
		1, onfail.Drop{}, &action.ZoneLayerChatSendAction{FromID: 1, Text: "x"},
	)
	for n := range data { // every proper prefix, which is bad after the state or before it
		env := aFreshEnv(c)
		if _, err := RestoreZoneLayerEngine(data[:n], env, 0); err == nil {
			t.Fatalf("%d of %d bytes were restored", n, len(data))
		}
		if reason, ok := env.Router.Deliver(say); ok || reason != message.NotBuilt {
			t.Fatalf("a checkpoint cut at %d of %d bytes registered an engine: %v, %v",
				n, len(data), reason, ok)
		}
	}
	env := aFreshEnv(c)
	if _, err := RestoreZoneLayerEngine(data, env, 0); err != nil {
		t.Fatal(err)
	}
	if reason, ok := env.Router.Deliver(say); !ok {
		t.Fatalf("a good checkpoint made no engine that a message reaches: %v", reason)
	}
}

// aBigZoneLayer is a zonelayer as full as the design says one gets: 150 characters, 150 items on
// the ground that will be taken off, and what each waits for.
func aBigZoneLayer(t testing.TB) *ZoneLayerEngine {
	t.Helper()
	c := aWorldToCheckpoint(t)
	l := c.ZoneLayer(1)
	for i := range 150 {
		ch, err := zonelayerstate.NewCharacter(state.CharacterID(i+1), "Binh", 100, 250, 256)
		if err != nil {
			t.Fatal(err)
		}
		ch.Cooldowns.Set(1, 40)
		at := state.Vec{X: int32(1000 + 40*i), Y: 2500}
		l.st.AddCharacterEntity(ch, at)
		l.st.AddItemEntity(state.ItemID(i%9+1), 3, at, 500)
		l.sched.At(
			uint64(500+i), &action.ZoneLayerItemEntityRemoveAction{EntityID: state.EntityID(i)},
		)
	}
	l.selfNext = []action.ZoneLayerAction{&action.ZoneLayerCharacterMoveAction{CharacterID: 1}}
	return l
}

func BenchmarkCheckpointZoneLayer(b *testing.B) {
	l := aBigZoneLayer(b)
	data, err := l.Checkpoint()
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := l.Checkpoint(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRestoreZoneLayer(b *testing.B) {
	l := aBigZoneLayer(b)
	data, err := l.Checkpoint()
	if err != nil {
		b.Fatal(err)
	}
	env := &Env{
		Router: NewRouter(), Cfg: l.cfg, ReplayLog: replay.NopLog{},
		Stable: NewStableStateRegistry(1), Graph: NewWorldGraph(1), QueueSize: 16,
		WorldEmitter: NopEmitter{}, ZoneEmitter: NopEmitter{}, ZoneLayerEmitter: NopEmitter{},
	}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := RestoreZoneLayerEngine(data, env, 0); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHashZoneLayerWithWhatItWaitsFor(b *testing.B) {
	l := aBigZoneLayer(b)
	b.ReportAllocs()
	for b.Loop() {
		l.hash()
	}
}

func BenchmarkHashZoneLayerState(b *testing.B) {
	l := aBigZoneLayer(b)
	b.ReportAllocs()
	for b.Loop() {
		l.st.Hash()
	}
}

// aBigWorld is a world as full as the design says one gets: 2,000 characters with a full inventory
// of 20 slots each, and the 1,000 private messages that it keeps.
func aBigWorld(t testing.TB) *WorldEngine {
	t.Helper()
	w := aWorldToCheckpoint(t).World
	for i := range 2000 {
		for item := range 20 {
			w.st.AddItemToCharacter(state.CharacterID(i+1), state.ItemID(item+1), 7, 20, 99)
		}
	}
	for i := range 1000 {
		w.st.SendPrivateMessage(worldstate.PrivateChatMessage{
			FromID: state.PlayerID(i), ToID: state.PlayerID(i + 1),
			Text: "xin chào, hôm nay thế nào", Tick: uint64(i),
		})
	}
	return w
}

func BenchmarkCheckpointWorld(b *testing.B) {
	w := aBigWorld(b)
	data, err := w.Checkpoint()
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := w.Checkpoint(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRestoreWorld(b *testing.B) {
	w := aBigWorld(b)
	data, err := w.Checkpoint()
	if err != nil {
		b.Fatal(err)
	}
	env := &Env{
		Router: NewRouter(), Cfg: w.cfg, ReplayLog: replay.NopLog{},
		Stable: NewStableStateRegistry(1), Graph: NewWorldGraph(1), QueueSize: 16,
		WorldEmitter: NopEmitter{}, ZoneEmitter: NopEmitter{}, ZoneLayerEmitter: NopEmitter{},
	}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := RestoreWorldEngine(data, env, 0); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHashWorld(b *testing.B) {
	w := aBigWorld(b)
	b.ReportAllocs()
	for b.Loop() {
		w.hash()
	}
}

// The clone that every engine publishes at the end of every tick, for comparison with what a
// checkpoint costs.
func BenchmarkCloneWorld(b *testing.B) {
	w := aBigWorld(b)
	b.ReportAllocs()
	for b.Loop() {
		w.st.Clone()
	}
}

func BenchmarkCloneZoneLayer(b *testing.B) {
	l := aBigZoneLayer(b)
	b.ReportAllocs()
	for b.Loop() {
		l.st.Clone()
	}
}

func TestEnsureRegenSetsTheFirstActionWhenNoneWaitsAndChecksTheOneThatDoes(t *testing.T) {
	c := aWorldToCheckpoint(t) // the builder asked for healing every 10 ticks
	l := c.ZoneLayer(1)
	key := action.ActionKey{ID: action.ZoneLayerCharacterRegenActionID}
	regen, ok := action.FindWaiting(&l.sched, l.selfNext, key)
	if !ok || regen.(*action.ZoneLayerCharacterRegenAction).Every != 10 {
		t.Fatalf("the zonelayer does not wait for the heal that was asked for: %v, %v", regen, ok)
	}
	if err := l.EnsureRegen(10); err != nil || l.sched.Pending() != 1 {
		t.Fatalf("the same period: error %v, %d waiting, want none added", err, l.sched.Pending())
	}
	if err := l.EnsureRegen(20); err == nil {
		t.Fatal("a period that is not the one that waits was accepted")
	}
	if err := l.EnsureRegen(0); err == nil {
		t.Fatal("no healing was accepted while a heal waits")
	}
	data, err := l.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	r, err := RestoreZoneLayerEngine(data, c.Env, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.EnsureRegen(10); err != nil || r.sched.Pending() != 1 {
		t.Fatalf("a restored zonelayer: error %v, %d waiting", err, r.sched.Pending())
	}
	if err := r.EnsureRegen(20); err == nil {
		t.Fatal("a restored zonelayer accepted another period than the one it was written with")
	}

	none := aWorldToCheckpoint(t).ZoneLayer(1)
	none.sched.Cancel(action.ActionKey{ID: action.ZoneLayerCharacterRegenActionID})
	if err := none.EnsureRegen(0); err != nil || none.sched.Pending() != 0 {
		t.Fatalf("no healing asked, none waiting: error %v, %d waiting", err, none.sched.Pending())
	}
	if err := none.EnsureRegen(5); err != nil || none.sched.Pending() != 1 {
		t.Fatalf("healing asked, none waiting: error %v, %d waiting", err, none.sched.Pending())
	}
}

func TestTheHashOfAnEngineCoversWhatOnlyTheEngineKnows(t *testing.T) {
	c := aWorldWithActionsThatWait(t)
	l := c.ZoneLayer(1)
	base := l.hash()
	changes := map[string]func(){
		"the number of its messages":          func() { l.seq++ },
		"the last number taken from a sender": func() { l.lastSeq[message.ZoneAddress(77)] = 5 },
		"the version of the configuration":    func() { l.cfgVersionSeen++ },
		"its random numbers":                  func() { l.rng.Uint64() },
		"a message it holds to send again": func() {
			l.bounce.pending = append(l.bounce.pending, action.MessageToZone(1, onfail.Drop{}))
		},
	}
	for name, change := range changes {
		change()
		now := l.hash()
		if now == base {
			t.Errorf("a change of %s did not change the hash", name)
		}
		base = now
	}
}
