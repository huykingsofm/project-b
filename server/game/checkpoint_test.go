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
	return aWorldToCheckpointWith(t, config.NewStore(), nil)
}

// aWorldToCheckpointWith is aWorldToCheckpoint with the configuration and the writer of the
// checkpoints given; with no writer the engines take none.
func aWorldToCheckpointWith(
	t testing.TB, cfg *config.Store, w engine.CheckpointWriter,
) *game.Cluster {
	t.Helper()
	tiles := tilemaptest.New(t, 10, 5, tilemap.Tile{X: 1, Y: 2}, nil, nil)
	c, err := game.NewBuilder().
		WithSeed(7).
		WithConfig(cfg).
		WithCheckpointWriter(w).
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
	testkit.Steps(t, c, 4)
	c.Env.Router.Deliver(sim.NewActionMessage[*zonelayerstate.State](
		theLayer, onfail.Drop{},
		&action.ZoneLayerItemEntityAddAction{
			ItemID: 7, Count: 1, At: state.Vec{X: 2500, Y: 2500}, Expiry: 40,
		},
	).Stamp(sim.Header{From: 99, Seq: 1}))
	l := c.ZoneLayer(theLayer)
	l.Send(command.ZoneLayerCharacterShieldGrantCommand{CharacterID: 1, Duration: 100})
	l.Send(command.ZoneLayerCharacterMoveCommand{CharacterID: 1, Dir: state.East, Ticks: 8})
	testkit.Steps(t, c, 3)
	c.Zone(testkit.Zone1).Send(command.ZoneChatSendCommand{FromID: 1, Text: "hello"})
	c.World.Send(command.WorldChatPrivateSendCommand{FromID: 1, ToID: 2, Text: "psst"})
	testkit.Steps(t, c, 2)
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
	data, err := l.Checkpoint().Encode()
	if err != nil {
		t.Fatal(err)
	}
	// in the environment of the cluster, as the engine it came from: it reads the zone as it does
	r, err := restoreEngine[*zonelayerstate.State](data, c.Env, 7)
	if err != nil {
		t.Fatal(err)
	}
	if hashOf(r) != hashOf(l) || r.Tick() != l.Tick() || r.State().Hash() != l.State().Hash() {
		t.Fatal("the restored engine has another hash, or another tick")
	}
	if again, err := r.Checkpoint().Encode(); err != nil || !bytes.Equal(again, data) {
		t.Fatalf("the restored engine is written differently: %v", err)
	}
	for tick := 1; tick <= 60; tick++ { // the item is taken off, the run ends, the heal comes twice
		l.Step()
		r.Step()
		if hashOf(r) != hashOf(l) {
			t.Fatalf("%d ticks after the restore the hash differs", tick)
		}
	}
}

func TestARestoredWorldAndZoneHaveTheHashOfTheOnesTheyCameFrom(t *testing.T) {
	c := aWorldWithActionsThatWait(t)
	world, err := c.World.Checkpoint().Encode()
	if err != nil {
		t.Fatal(err)
	}
	w, err := restoreEngine[*worldstate.State](world, aFreshEnv(c), 7)
	if err != nil || hashOf(w) != hashOf(c.World) || w.Tick() != c.World.Tick() {
		t.Fatalf("the world: %v", err)
	}
	if len(w.State().ZoneIDs()) != 1 || len(w.State().ZoneLayerIDs()) != 1 {
		t.Fatalf("the restored world knows %+v", w.State())
	}
	if again, err := w.Checkpoint().Encode(); err != nil || !bytes.Equal(again, world) {
		t.Fatalf("the restored world is written differently: %v", err)
	}
	zone, err := c.Zone(testkit.Zone1).Checkpoint().Encode()
	if err != nil {
		t.Fatal(err)
	}
	z, err := restoreEngine[*zonestate.State](zone, aFreshEnv(c), 7)
	if err != nil || hashOf(z) != hashOf(c.Zone(testkit.Zone1)) ||
		z.Tick() != c.Zone(testkit.Zone1).Tick() {
		t.Fatalf("the zone: %v", err)
	}
	if again, err := z.Checkpoint().Encode(); err != nil || !bytes.Equal(again, zone) {
		t.Fatalf("the restored zone is written differently: %v", err)
	}
	zoneState := c.Zone(testkit.Zone1).State()
	if c.World.State().PrivateMessageCount() == 0 || zoneState.ChatMessageCount() == 0 {
		t.Fatal("the world and the zone hold nothing to restore: the test checks nothing")
	}
}

func TestACheckpointThatIsCutOrOfAnotherTypeOfStateIsAnErrorAndNeverAPanic(t *testing.T) {
	c := aWorldWithActionsThatWait(t)
	data, err := c.ZoneLayer(theLayer).Checkpoint().Encode()
	if err != nil {
		t.Fatal(err)
	}
	for n := range data { // every proper prefix
		_, err := restoreEngine[*zonelayerstate.State](data[:n], aFreshEnv(c), 7)
		if err == nil {
			t.Fatalf("%d of %d bytes were restored", n, len(data))
		}
	}
	longer := append(append([]byte{}, data...), 0)
	if _, err := restoreEngine[*zonelayerstate.State](longer, aFreshEnv(c), 7); err == nil {
		t.Fatal("a checkpoint with a byte more was restored")
	}
	if _, err := restoreEngine[*worldstate.State](data, aFreshEnv(c), 7); err == nil {
		t.Fatal("the checkpoint of a zonelayer was restored as the world")
	}
	if _, err := restoreEngine[*zonestate.State](data, aFreshEnv(c), 7); err == nil {
		t.Fatal("the checkpoint of a zonelayer was restored as a zone")
	}
}

// A checkpoint that is bad makes no engine, and so registers none in the router: a message to it
// bounces as an engine that is not built, and does not wait in a queue that nothing reads.
func TestACheckpointThatIsBadRegistersNoEngineInTheRouter(t *testing.T) {
	c := aWorldWithActionsThatWait(t)
	data, err := c.ZoneLayer(theLayer).Checkpoint().Encode()
	if err != nil {
		t.Fatal(err)
	}
	say := sim.NewActionMessage[*zonelayerstate.State](
		theLayer, onfail.Drop{}, &action.ZoneLayerChatSendAction{FromID: 1, Text: "x"},
	)
	for n := range data { // every proper prefix, which is bad after the state or before it
		env := aFreshEnv(c)
		if _, err := restoreEngine[*zonelayerstate.State](data[:n], env, 7); err == nil {
			t.Fatalf("%d of %d bytes were restored", n, len(data))
		}
		if reason, ok := env.Router.Deliver(say); ok || reason != sim.DeliveryFailureNotStarted {
			t.Fatalf("a checkpoint cut at %d of %d bytes registered an engine: %v, %v",
				n, len(data), reason, ok)
		}
	}
	env := aFreshEnv(c)
	if _, err := restoreEngine[*zonelayerstate.State](data, env, 7); err != nil {
		t.Fatal(err)
	}
	if reason, ok := env.Router.Deliver(say); !ok {
		t.Fatalf("a good checkpoint made no engine that a message reaches: %v", reason)
	}
}

// aBigZoneLayer is a zonelayer as full as the design says one gets: 150 characters, 150 items on
// the ground that will be taken off, and what each waits for.
func aBigZoneLayer(t testing.TB) *engine.Engine[*zonelayerstate.State] {
	t.Helper()
	return aBigZoneLayerOf(t, aWorldToCheckpoint(t))
}

// aBigZoneLayerOf is aBigZoneLayer, made the zonelayer of the world given.
func aBigZoneLayerOf(t testing.TB, c *game.Cluster) *engine.Engine[*zonelayerstate.State] {
	t.Helper()
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
				EntityID: state.NewEntityID(theLayer, uint64(i+1)),
			},
		)
	}
	return l
}

// BenchmarkEncodeCheckpointZoneLayer is what the writer of the checkpoints pays for one: the
// encoding of a checkpoint that was taken.
func BenchmarkEncodeCheckpointZoneLayer(b *testing.B) {
	taken := aBigZoneLayer(b).Checkpoint()
	data, err := taken.Encode()
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := taken.Encode(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTakeCheckpointZoneLayer is a checkpoint taken between two ticks: a clone of the state,
// and of what the engine waits for and what only it and its peer know.
func BenchmarkTakeCheckpointZoneLayer(b *testing.B) {
	l := aBigZoneLayer(b)
	b.ReportAllocs()
	for b.Loop() {
		l.Checkpoint()
	}
}

func BenchmarkRestoreZoneLayer(b *testing.B) {
	c := aWorldToCheckpoint(b)
	l := aBigZoneLayer(b)
	data, err := l.Checkpoint().Encode()
	if err != nil {
		b.Fatal(err)
	}
	env := aFreshEnv(c)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := restoreEngine[*zonelayerstate.State](data, env, 7); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHashZoneLayerWithWhatItWaitsFor(b *testing.B) {
	l := aBigZoneLayer(b)
	b.ReportAllocs()
	for b.Loop() {
		hashOf(l)
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
	return aBigWorldOf(t, aWorldToCheckpoint(t))
}

// aBigWorldOf is aBigWorld, made the world given.
func aBigWorldOf(t testing.TB, c *game.Cluster) *engine.Engine[*worldstate.State] {
	t.Helper()
	w := c.World
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

// BenchmarkEncodeCheckpointWorld is what the writer of the checkpoints pays for one: the
// encoding of a checkpoint that was taken.
func BenchmarkEncodeCheckpointWorld(b *testing.B) {
	taken := aBigWorld(b).Checkpoint()
	data, err := taken.Encode()
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := taken.Encode(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTakeCheckpointWorld is a checkpoint taken between two ticks: a clone of the state, and
// of what the engine waits for and what only it and its peer know.
func BenchmarkTakeCheckpointWorld(b *testing.B) {
	w := aBigWorld(b)
	b.ReportAllocs()
	for b.Loop() {
		w.Checkpoint()
	}
}

func BenchmarkRestoreWorld(b *testing.B) {
	c := aWorldToCheckpoint(b)
	w := aBigWorld(b)
	data, err := w.Checkpoint().Encode()
	if err != nil {
		b.Fatal(err)
	}
	env := aFreshEnv(c)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := restoreEngine[*worldstate.State](data, env, 7); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHashWorld(b *testing.B) {
	w := aBigWorld(b)
	b.ReportAllocs()
	for b.Loop() {
		hashOf(w)
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

// discardingWriter is a writer of checkpoints that takes each and writes none: what is left of
// taking a checkpoint is what the goroutine of the engine pays for it.
type discardingWriter struct{}

func (discardingWriter) Offer(sim.StateID, uint64, func() ([]byte, error)) {}

// stepping is the world of aWorldToCheckpoint that takes a checkpoint at every tick, or at none.
func stepping(b *testing.B, checkpointAtEveryTick bool) *game.Cluster {
	b.Helper()
	cfg := config.NewStore()
	if !checkpointAtEveryTick {
		return aWorldToCheckpointWith(b, cfg, nil)
	}
	if _, err := cfg.Apply(map[string]string{"checkpoint.every_ticks": "1"}); err != nil {
		b.Fatal(err)
	}
	return aWorldToCheckpointWith(b, cfg, discardingWriter{})
}

// benchmarkStep measures a tick of an engine with nothing sent to it: the clone it publishes, the
// hash, and taking a checkpoint when the world takes one at every tick.
func benchmarkStep(b *testing.B, step func() error) {
	b.ReportAllocs()
	for b.Loop() {
		if err := step(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStepZoneLayer(b *testing.B) {
	benchmarkStep(b, aBigZoneLayerOf(b, stepping(b, false)).Step)
}

func BenchmarkStepZoneLayerTakingACheckpoint(b *testing.B) {
	benchmarkStep(b, aBigZoneLayerOf(b, stepping(b, true)).Step)
}

func BenchmarkStepWorld(b *testing.B) {
	benchmarkStep(b, aBigWorldOf(b, stepping(b, false)).Step)
}

func BenchmarkStepWorldTakingACheckpoint(b *testing.B) {
	benchmarkStep(b, aBigWorldOf(b, stepping(b, true)).Step)
}

// hashOf is the hash of an engine, from a new hasher.
func hashOf[S sim.State](e *engine.Engine[S]) uint64 { return e.Hash(sim.NewHasher()) }

// restoreEngine reads a checkpoint and makes its engine, in the environment given.
func restoreEngine[S sim.State](
	data []byte, env *engine.Env, seed uint64,
) (*engine.Engine[S], error) {
	c, err := engine.DecodeEngineCheckpoint[S](data, env.Codecs)
	if err != nil {
		return nil, err
	}
	return c.Restore(env, seed)
}
