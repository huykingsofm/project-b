package game_test

import (
	"bytes"
	"testing"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/server/game"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/command"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap/tilemaptest"
	"github.com/huykingsofm/project-b/server/game/testkit"
)

// aWorldToCheckpoint is a world of one zone with one zonelayer, that heals its characters every 10
// ticks.
func aWorldToCheckpoint(t testing.TB) *game.Cluster {
	t.Helper()
	tiles := tilemaptest.New(t, 10, 5, tilemap.Tile{X: 1, Y: 2}, nil, nil)
	c, err := game.NewBuilder().
		WithSeed(7).
		WithReplayLog(&replay.MemLog{}).
		WithWorld(game.WorldParams{ID: testkit.World}).
		WithZone(game.ZoneParams{ID: testkit.Zone1, Tiles: tiles}).
		WithZoneLayers(game.ZoneLayerParams{
			ZoneID: testkit.Zone1, Count: 1, Capacity: 5, RegenEvery: 10,
		}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// theLayer is the only zonelayer of aWorldToCheckpoint: the first of its type.
var theLayer = testkit.Layer1

// aWorldWithActionsThatWait has a character that runs and is shielded, and an item on the ground
// that will be taken off: so the zonelayer waits for things at a later tick and has set things for
// itself for the next one.
func aWorldWithActionsThatWait(t testing.TB) *game.Cluster {
	t.Helper()
	c := aWorldToCheckpoint(t)
	an, err := zonelayerstate.NewCharacter(1, "An", 100, 1000, 0)
	if err != nil {
		t.Fatal(err)
	}
	c.Zone(testkit.Zone1).Send(command.ZoneCharacterEnterCommand{Character: an})
	testkit.Steps(c, 4)
	c.Env.Router.Deliver(sim.NewActionMessage[*zonelayerstate.State](
		theLayer, onfail.Drop{},
		&action.ZoneLayerItemEntityAddAction{
			ItemID: 7, Count: 1, At: state.Vec{X: 2500, Y: 2500}, Expiry: 40,
		},
	).Stamp(sim.Header{From: 99, Seq: 1}))
	l := c.ZoneLayer(theLayer)
	l.Send(command.ZoneLayerCharacterShieldGrantCommand{CharacterID: 1, Duration: 100})
	l.Send(command.ZoneLayerCharacterMoveCommand{CharacterID: 1, Dir: state.East, Ticks: 8})
	testkit.Steps(c, 3)
	c.Zone(testkit.Zone1).Send(command.ZoneChatSendCommand{FromID: 1, Text: "hello"})
	c.World.Send(command.WorldChatPrivateSendCommand{FromID: 1, ToID: 2, Text: "psst"})
	testkit.Steps(c, 2)
	return c
}

// aFreshEnv is an environment with a router, a store and a configuration of its own, in which no
// engine has been made, and with the codecs of the cluster.
func aFreshEnv(c *game.Cluster) *engine.Env {
	return &engine.Env{
		Router: engine.NewRouter(), Cfg: config.NewStore(), ReplayLog: replay.NopLog{},
		Stable: engine.NewStableStateStore(), Codecs: c.Env.Codecs, QueueSize: 16,
		Emitter: engine.NopEmitter{},
	}
}

func TestARestoredZoneLayerHasTheHashOfTheOneItCameFromAndGoesOnTheSame(t *testing.T) {
	c := aWorldWithActionsThatWait(t)
	l := c.ZoneLayer(theLayer)
	if _, waits := l.Scheduler().FindByID(action.ZoneLayerCharacterRegenActionID); !waits {
		t.Fatal("the zonelayer waits for no heal: the test checks nothing")
	}
	data, err := l.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	// in the environment of the cluster, as the engine it came from: it reads the zone as it does
	r, err := engine.Restore[*zonelayerstate.State](data, c.Env, 7)
	if err != nil {
		t.Fatal(err)
	}
	if r.Hash() != l.Hash() || r.Tick() != l.Tick() || r.State().Hash() != l.State().Hash() {
		t.Fatal("the restored engine has another hash, or another tick")
	}
	if again, err := r.Checkpoint(); err != nil || !bytes.Equal(again, data) {
		t.Fatalf("the restored engine is written differently: %v", err)
	}
	for tick := 1; tick <= 60; tick++ { // the item is taken off, the run ends, the heal comes twice
		l.Step()
		r.Step()
		if r.Hash() != l.Hash() {
			t.Fatalf("%d ticks after the restore the hash differs", tick)
		}
	}
}

func TestARestoredWorldAndZoneHaveTheHashOfTheOnesTheyCameFrom(t *testing.T) {
	c := aWorldWithActionsThatWait(t)
	world, err := c.World.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	w, err := engine.Restore[*worldstate.State](world, aFreshEnv(c), 7)
	if err != nil || w.Hash() != c.World.Hash() || w.Tick() != c.World.Tick() {
		t.Fatalf("the world: %v", err)
	}
	if len(w.State().ZoneIDs()) != 1 || len(w.State().ZoneLayerIDs()) != 1 {
		t.Fatalf("the restored world knows %+v", w.State())
	}
	if again, err := w.Checkpoint(); err != nil || !bytes.Equal(again, world) {
		t.Fatalf("the restored world is written differently: %v", err)
	}
	zone, err := c.Zone(testkit.Zone1).Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	z, err := engine.Restore[*zonestate.State](zone, aFreshEnv(c), 7)
	if err != nil || z.Hash() != c.Zone(testkit.Zone1).Hash() ||
		z.Tick() != c.Zone(testkit.Zone1).Tick() {
		t.Fatalf("the zone: %v", err)
	}
	if again, err := z.Checkpoint(); err != nil || !bytes.Equal(again, zone) {
		t.Fatalf("the restored zone is written differently: %v", err)
	}
	zoneState := c.Zone(testkit.Zone1).State()
	if c.World.State().PrivateMessageCount() == 0 || zoneState.ChatMessageCount() == 0 {
		t.Fatal("the world and the zone hold nothing to restore: the test checks nothing")
	}
}

func TestACheckpointThatIsCutOrOfAnotherTypeOfStateIsAnErrorAndNeverAPanic(t *testing.T) {
	c := aWorldWithActionsThatWait(t)
	data, err := c.ZoneLayer(theLayer).Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	for n := range data { // every proper prefix
		if _, err := engine.Restore[*zonelayerstate.State](data[:n], aFreshEnv(c), 7); err == nil {
			t.Fatalf("%d of %d bytes were restored", n, len(data))
		}
	}
	longer := append(append([]byte{}, data...), 0)
	if _, err := engine.Restore[*zonelayerstate.State](longer, aFreshEnv(c), 7); err == nil {
		t.Fatal("a checkpoint with a byte more was restored")
	}
	if _, err := engine.Restore[*worldstate.State](data, aFreshEnv(c), 7); err == nil {
		t.Fatal("the checkpoint of a zonelayer was restored as the world")
	}
	if _, err := engine.Restore[*zonestate.State](data, aFreshEnv(c), 7); err == nil {
		t.Fatal("the checkpoint of a zonelayer was restored as a zone")
	}
}

// A checkpoint that is bad makes no engine, and so registers none in the router: a message to it
// bounces as an engine that is not built, and does not wait in a queue that nothing reads.
func TestACheckpointThatIsBadRegistersNoEngineInTheRouter(t *testing.T) {
	c := aWorldWithActionsThatWait(t)
	data, err := c.ZoneLayer(theLayer).Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	say := sim.NewActionMessage[*zonelayerstate.State](
		theLayer, onfail.Drop{}, &action.ZoneLayerChatSendAction{FromID: 1, Text: "x"},
	)
	for n := range data { // every proper prefix, which is bad after the state or before it
		env := aFreshEnv(c)
		if _, err := engine.Restore[*zonelayerstate.State](data[:n], env, 7); err == nil {
			t.Fatalf("%d of %d bytes were restored", n, len(data))
		}
		if reason, ok := env.Router.Deliver(say); ok || reason != sim.NotBuilt {
			t.Fatalf("a checkpoint cut at %d of %d bytes registered an engine: %v, %v",
				n, len(data), reason, ok)
		}
	}
	env := aFreshEnv(c)
	if _, err := engine.Restore[*zonelayerstate.State](data, env, 7); err != nil {
		t.Fatal(err)
	}
	if reason, ok := env.Router.Deliver(say); !ok {
		t.Fatalf("a good checkpoint made no engine that a message reaches: %v", reason)
	}
}

func TestEnsureRegenSetsTheFirstActionWhenNoneWaitsAndChecksTheOneThatDoes(t *testing.T) {
	c := aWorldToCheckpoint(t) // the builder asked for healing every 10 ticks
	l := c.ZoneLayer(theLayer)
	regen, ok := l.Scheduler().FindByID(action.ZoneLayerCharacterRegenActionID)
	if !ok || regen.(*action.ZoneLayerCharacterRegenAction).Every != 10 {
		t.Fatalf("the zonelayer does not wait for the heal that was asked for: %v, %v", regen, ok)
	}
	if err := game.EnsureRegen(l, 10); err != nil {
		t.Fatalf("the same period: %v", err)
	}
	if err := game.EnsureRegen(l, 20); err == nil {
		t.Fatal("a period that is not the one that waits was accepted")
	}
	if err := game.EnsureRegen(l, 0); err == nil {
		t.Fatal("no healing was accepted while a heal waits")
	}
	data, err := l.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	r, err := engine.Restore[*zonelayerstate.State](data, c.Env, 7)
	if err != nil {
		t.Fatal(err)
	}
	if err := game.EnsureRegen(r, 10); err != nil {
		t.Fatalf("a restored zonelayer: %v", err)
	}
	if err := game.EnsureRegen(r, 20); err == nil {
		t.Fatal("a restored zonelayer accepted another period than the one it was written with")
	}

	// a builder that asks for no healing leaves nothing waiting, and asking for one sets the first
	noHeal, err := game.NewBuilder().
		WithWorld(game.WorldParams{ID: testkit.World}).
		WithZone(game.ZoneParams{
			ID: testkit.Zone1, Tiles: tilemaptest.New(t, 10, 5, tilemap.Tile{}, nil, nil),
		}).
		WithZoneLayers(game.ZoneLayerParams{ZoneID: testkit.Zone1, Count: 1, Capacity: 1}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	none := noHeal.ZoneLayer(theLayer)
	if _, waits := none.Scheduler().FindByID(action.ZoneLayerCharacterRegenActionID); waits {
		t.Fatal("a zonelayer that heals every 0 ticks waits for a heal")
	}
	if err := game.EnsureRegen(none, 0); err != nil {
		t.Fatalf("no healing asked, none waiting: %v", err)
	}
	if err := game.EnsureRegen(none, 5); err != nil {
		t.Fatalf("healing asked, none waiting: %v", err)
	}
	if _, waits := none.Scheduler().FindByID(action.ZoneLayerCharacterRegenActionID); !waits {
		t.Fatal("asking for healing set no heal")
	}
}

// aBigZoneLayer is a zonelayer as full as the design says one gets: 150 characters, 150 items on
// the ground that will be taken off, and what each waits for.
func aBigZoneLayer(t testing.TB) *engine.Engine[*zonelayerstate.State] {
	t.Helper()
	c := aWorldToCheckpoint(t)
	l := c.ZoneLayer(theLayer)
	for i := range 150 {
		ch, err := zonelayerstate.NewCharacter(state.CharacterID(i+1), "Binh", 100, 250, 256)
		if err != nil {
			t.Fatal(err)
		}
		ch.SetCooldown(1, 40)
		at := state.Vec{X: int32(1000 + 40*i), Y: 2500}
		l.State().AddCharacterEntity(ch, at)
		l.State().AddItemEntity(state.ItemID(i%9+1), 3, at, 500)
		l.ScheduleAt(
			uint64(500+i),
			&action.ZoneLayerItemEntityRemoveAction{
				EntityID: state.NewEntityID(theLayer, uint32(i)),
			},
		)
	}
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
	c := aWorldToCheckpoint(b)
	l := aBigZoneLayer(b)
	data, err := l.Checkpoint()
	if err != nil {
		b.Fatal(err)
	}
	env := aFreshEnv(c)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := engine.Restore[*zonelayerstate.State](data, env, 7); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHashZoneLayerWithWhatItWaitsFor(b *testing.B) {
	l := aBigZoneLayer(b)
	b.ReportAllocs()
	for b.Loop() {
		l.Hash()
	}
}

func BenchmarkHashZoneLayerState(b *testing.B) {
	l := aBigZoneLayer(b)
	b.ReportAllocs()
	for b.Loop() {
		l.State().Hash()
	}
}

// aBigWorld is a world as full as the design says one gets: 2,000 characters with a full inventory
// of 20 slots each, and the 1,000 private messages that it keeps.
func aBigWorld(t testing.TB) *engine.Engine[*worldstate.State] {
	t.Helper()
	w := aWorldToCheckpoint(t).World
	for i := range 2000 {
		for item := range 20 {
			w.State().AddItemToCharacter(state.CharacterID(i+1), state.ItemID(item+1), 7, 20, 99)
		}
	}
	for i := range 1000 {
		w.State().SendPrivateMessage(worldstate.NewPrivateChatMessage(
			state.PlayerID(i), state.PlayerID(i+1), "xin chào, hôm nay thế nào", uint64(i),
		))
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
	c := aWorldToCheckpoint(b)
	w := aBigWorld(b)
	data, err := w.Checkpoint()
	if err != nil {
		b.Fatal(err)
	}
	env := aFreshEnv(c)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := engine.Restore[*worldstate.State](data, env, 7); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHashWorld(b *testing.B) {
	w := aBigWorld(b)
	b.ReportAllocs()
	for b.Loop() {
		w.Hash()
	}
}

// The clone that every engine publishes at the end of every tick, for comparison with what a
// checkpoint costs.
func BenchmarkCloneWorld(b *testing.B) {
	w := aBigWorld(b)
	b.ReportAllocs()
	for b.Loop() {
		w.State().Clone()
	}
}

func BenchmarkCloneZoneLayer(b *testing.B) {
	l := aBigZoneLayer(b)
	b.ReportAllocs()
	for b.Loop() {
		l.State().Clone()
	}
}
