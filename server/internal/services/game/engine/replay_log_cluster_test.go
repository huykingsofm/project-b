package engine_test

import (
	"context"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"reflect"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/command"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap/tilemaptest"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// teeReplayLog gives every record to two replay logs.
type teeReplayLog struct{ a, b replay.Log }

func (t teeReplayLog) Inbound(e action.Address, tick uint64, h action.Header, ids []action.ID) {
	t.a.Inbound(e, tick, h, ids)
	t.b.Inbound(e, tick, h, ids)
}
func (t teeReplayLog) WorldCommand(e action.Address, tick uint64, c command.WorldCommand) {
	t.a.WorldCommand(e, tick, c)
	t.b.WorldCommand(e, tick, c)
}
func (t teeReplayLog) ZoneCommand(e action.Address, tick uint64, c command.ZoneCommand) {
	t.a.ZoneCommand(e, tick, c)
	t.b.ZoneCommand(e, tick, c)
}
func (t teeReplayLog) ZoneLayerCommand(e action.Address, tick uint64, c command.ZoneLayerCommand) {
	t.a.ZoneLayerCommand(e, tick, c)
	t.b.ZoneLayerCommand(e, tick, c)
}
func (t teeReplayLog) ConfigRead(e action.Address, tick uint64, v uint64) {
	t.a.ConfigRead(e, tick, v)
	t.b.ConfigRead(e, tick, v)
}
func (t teeReplayLog) Hash(e action.Address, tick uint64, hash func() uint64) {
	t.a.Hash(e, tick, hash)
	t.b.Hash(e, tick, hash)
}
func (t teeReplayLog) StableRead(e action.Address, tick uint64, from action.Address, copyTick uint64) {
	t.a.StableRead(e, tick, from, copyTick)
	t.b.StableRead(e, tick, from, copyTick)
}
func (t teeReplayLog) EndTick(e action.Address, tick uint64) {
	t.a.EndTick(e, tick)
	t.b.EndTick(e, tick)
}

var clusterEngines = []action.Address{
	action.WorldAddress(1),
	action.ZoneAddress(1), action.ZoneAddress(2),
	action.ZoneLayerAddress(1), action.ZoneLayerAddress(2), action.ZoneLayerAddress(3),
}

// newTeeWorld is newWorld with a replay log that writes the records both in memory and to files.
func newTeeWorld(t *testing.T, dir string) (*engine.Cluster, *replay.MemLog, *replay.FileLog, *config.Store) {
	t.Helper()
	exit1 := tilemap.ExitDefinition{Min: tilemap.Tile{X: 9, Y: 0}, Max: tilemap.Tile{X: 9, Y: 4}, ToZoneID: 2, Arrival: tilemap.Tile{X: 1, Y: 2}}
	exit2 := tilemap.ExitDefinition{Min: tilemap.Tile{X: 0, Y: 0}, Max: tilemap.Tile{X: 0, Y: 4}, ToZoneID: 1, Arrival: tilemap.Tile{X: 8, Y: 2}}
	spawn := tilemap.Tile{X: 1, Y: 2}
	mem := &replay.MemLog{}
	file := replay.NewFileLog(dir, replay.FileLogOptions{Seed: 7, SegmentBytes: 2048, SyncEvery: time.Hour, Backlog: 8})
	cfg := config.NewStore()
	c, err := engine.NewBuilder().
		WithConfig(cfg).
		WithSeed(7).
		WithReplayLog(teeReplayLog{mem, file}).
		WithWorld(engine.WorldParams{ID: 1}).
		WithZone(engine.ZoneParams{ID: 1, Tiles: tilemaptest.New(t, 10, 5, spawn, nil, []tilemap.ExitDefinition{exit1}), Adjacent: []state.ZoneID{2}}).
		WithZoneLayers(engine.ZoneLayerParams{ZoneID: 1, Count: 2, Capacity: 2, RegenEvery: 10}).
		WithZone(engine.ZoneParams{ID: 2, Tiles: tilemaptest.New(t, 10, 5, spawn, nil, []tilemap.ExitDefinition{exit2}), Adjacent: []state.ZoneID{1}}).
		WithZoneLayers(engine.ZoneLayerParams{ZoneID: 2, Count: 1, Capacity: 1, RegenEvery: 10}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	return c, mem, file, cfg
}

// recordsOf returns the records of the memory log that belong to an engine, in order, with an empty list of
// actions made nil as the file log reads it.
func recordsOf(mem *replay.MemLog, e action.Address) []replay.Record {
	var out []replay.Record
	for _, r := range mem.Records {
		if r.Engine != e {
			continue
		}
		if len(r.Actions) == 0 {
			r.Actions = nil
		}
		out = append(out, r)
	}
	return out
}

// compareWithFiles reads the log of every engine and checks that it is the memory log of that engine.
func compareWithFiles(t *testing.T, dir string, mem *replay.MemLog) {
	t.Helper()
	kinds := map[replay.Kind]int{}
	for _, e := range clusterEngines {
		want := recordsOf(mem, e)
		r, err := replay.ReadFileLog(dir, e, 0)
		if err != nil {
			t.Fatal(err)
		}
		var got []replay.Record
		for r.Next() {
			got = append(got, r.Record())
		}
		if err := r.Err(); err != nil {
			t.Fatalf("%v: %v", e, err)
		}
		r.Close()
		if len(want) == 0 {
			t.Fatalf("%v recorded nothing: the scenario does not reach it", e)
		}
		if !reflect.DeepEqual(got, want) {
			for i := range min(len(got), len(want)) {
				if !reflect.DeepEqual(got[i], want[i]) {
					t.Fatalf("%v: record %d differs:\n log    %+v\n memory %+v", e, i, got[i], want[i])
				}
			}
			t.Fatalf("%v: the log has %d records and memory has %d", e, len(got), len(want))
		}
		for _, rec := range got {
			kinds[rec.Kind]++
		}
	}
	for _, k := range []replay.Kind{replay.KindInbound, replay.KindCommand, replay.KindConfig, replay.KindHash, replay.KindStableRead} {
		if kinds[k] == 0 {
			t.Errorf("no record of kind %d in the scenario", k)
		}
	}
}

// sender gives the commands of a scenario to the engines as they are, or after they were written to bytes and
// read back: a copy that shares nothing with what the caller made, as a command that came from a client does.
type sender struct {
	t         *testing.T
	roundTrip bool
	reuses    bool // the sender goes on using the value it sent, and changes it
}

func via[C command.Command](s sender, c C) C {
	if !s.roundTrip {
		return c
	}
	var w binarycodec.Writer
	if err := command.Encode(&w, c); err != nil {
		s.t.Fatal(err)
	}
	got, err := command.Decode(binarycodec.NewReader(w.B), 1)
	if err != nil {
		s.t.Fatal(err)
	}
	return got.(C)
}

// runScenario runs a cluster through chat in each scope, a shield, an attack, a pickup, a run of movement, a new
// version of the config, a character that enters with cooldowns and one that crosses to another zone, long
// enough for every engine to record a hash.
func runScenario(t *testing.T, c *engine.Cluster, cfg *config.Store, s sender) {
	t.Helper()
	an := newCharacter(1, "An", 100, 1000, 0)
	an.Cooldowns.Set(action.ZoneLayerCharacterNormalAttackTriggerActionCooldownKey, 9)
	c.Zone(1).Send(via(s, command.ZoneCharacterEnterCommand{Character: an}))
	if s.reuses {
		an.Cooldowns.Set(action.ZoneLayerCharacterNormalAttackTriggerActionCooldownKey, 1<<40)
	}
	steps(c, 4)
	c.Zone(1).Send(via(s, command.ZoneCharacterEnterCommand{Character: newCharacter(2, "Binh", 100, 1000, 0)}))
	steps(c, 4)
	itemID := c.ZoneLayer(1).State().AddItemEntity(7, 1, state.Vec{X: 2500, Y: 2500}, 0)

	l := c.ZoneLayer(1)
	c.World.Send(via(s, command.WorldChatPrivateSendCommand{FromID: 10, ToID: 20, Text: "psst"}))
	c.Zone(1).Send(via(s, command.ZoneChatSendCommand{FromID: 1, Text: "hello zone"}))
	l.Send(via(s, command.ZoneLayerChatSendCommand{FromID: 1, Text: "xin chào"}))
	l.Send(via(s, command.ZoneLayerCharacterShieldGrantCommand{CharacterID: 2, Duration: 100}))
	l.Send(via(s, command.ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID: 1, TargetID: 2}))
	l.Send(via(s, command.ZoneLayerCharacterItemPickupCommand{CharacterID: 1, ItemEntityID: itemID}))
	steps(c, 6)
	if _, err := cfg.Apply(map[string]string{"zonelayer.normal_attack.cooldown_ticks": "5"}); err != nil { // a new version of the config: the engines record that they read it
		t.Fatal(err)
	}
	l.Send(via(s, command.ZoneLayerCharacterMoveCommand{CharacterID: 1, Dir: state.East, Ticks: 8}))
	steps(c, 14)
	l.Send(via(s, command.ZoneLayerCharacterExitToCommand{CharacterID: 1, ExitEntityID: 1}))
	steps(c, 60) // the zone it went to records a hash too
}

func TestTheFilesOfAWholeClusterRunHoldWhatTheMemoryLogHolds(t *testing.T) {
	dir := t.TempDir()
	c, mem, file, cfg := newTeeWorld(t, dir)
	runScenario(t, c, cfg, sender{t: t})
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	compareWithFiles(t, dir, mem)
}

// The engines must not depend on memory they share with the sender of a command. The sender here goes on using the
// character it sent and changes its cooldowns; the same run with every command written to bytes and read back, which
// shares nothing with the sender, must give the same hash at every tick that records one and the same state at the
// end. (The memory log keeps a command as the sender gave it, so its command records are not compared.)
func TestACommandThatIsCopiedGivesTheSameRunAsOneThatIsNot(t *testing.T) {
	run := func(roundTrip bool) ([]replay.Record, []uint64) {
		c, _, mem, cfg := newWorld(t, 7, 10)
		runScenario(t, c, cfg, sender{t: t, roundTrip: roundTrip, reuses: true})
		var hashes []replay.Record
		for _, r := range mem.Records {
			if r.Kind == replay.KindHash {
				hashes = append(hashes, r)
			}
		}
		return hashes, []uint64{
			c.World.State().Hash(), c.Zone(1).State().Hash(), c.Zone(2).State().Hash(),
			c.ZoneLayer(1).State().Hash(), c.ZoneLayer(2).State().Hash(), c.ZoneLayer(3).State().Hash(),
		}
	}
	directHashes, directFinal := run(false)
	copiedHashes, copiedFinal := run(true)
	if len(directHashes) == 0 {
		t.Fatal("the scenario recorded no hash")
	}
	if !reflect.DeepEqual(directHashes, copiedHashes) {
		t.Fatalf("the hashes recorded differ:\n direct %v\n copied %v", directHashes, copiedHashes)
	}
	if !reflect.DeepEqual(directFinal, copiedFinal) {
		t.Fatalf("the state at the end differs: %v and %v", directFinal, copiedFinal)
	}
}

func TestTheFilesOfEnginesThatRunOnTheirOwnGoroutinesHoldWhatTheMemoryLogHolds(t *testing.T) {
	dir := t.TempDir()
	c, mem, file, _ := newTeeWorld(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx, time.Millisecond); close(done) }()

	for i := range 30 {
		c.Zone(1).Send(command.ZoneCharacterEnterCommand{Character: newCharacter(state.CharacterID(i+1), "x", 100, 1000, 0)})
		c.World.Send(command.WorldChatPrivateSendCommand{FromID: 1, ToID: 2, Text: "psst"})
		time.Sleep(2 * time.Millisecond)
	}
	time.Sleep(60 * time.Millisecond)
	cancel()
	<-done
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	for _, e := range []action.Address{action.WorldAddress(1), action.ZoneAddress(1), action.ZoneLayerAddress(1)} {
		want := recordsOf(mem, e)
		r, err := replay.ReadFileLog(dir, e, 0)
		if err != nil {
			t.Fatal(err)
		}
		var got []replay.Record
		for r.Next() {
			got = append(got, r.Record())
		}
		if r.Err() != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%v: %d records in the log, %d in memory, error %v", e, len(got), len(want), r.Err())
		}
		r.Close()
	}
}
