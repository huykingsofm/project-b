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

func TestTheFilesOfAWholeClusterRunHoldWhatTheMemoryLogHolds(t *testing.T) {
	dir := t.TempDir()
	c, mem, file, cfg := newTeeWorld(t, dir)

	an := newCharacter(1, "An", 100, 1000, 0)
	an.Cooldowns.Set(action.ZoneLayerCharacterNormalAttackTriggerActionCooldownKey, 9)
	enterCharacter(t, c, 1, an)
	enter(t, c, 1, 2, "Binh")
	itemID := c.ZoneLayer(1).State().AddItemEntity(7, 1, state.Vec{X: 2500, Y: 2500}, 0)

	l := c.ZoneLayer(1)
	c.World.Send(command.WorldChatPrivateSendCommand{FromID: 10, ToID: 20, Text: "psst"})
	c.Zone(1).Send(command.ZoneChatSendCommand{FromID: 1, Text: "hello zone"})
	l.Send(command.ZoneLayerChatSendCommand{FromID: 1, Text: "xin chào"})
	l.Send(command.ZoneLayerCharacterShieldGrantCommand{CharacterID: 2, Duration: 100})
	l.Send(command.ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID: 1, TargetID: 2})
	l.Send(command.ZoneLayerCharacterItemPickupCommand{CharacterID: 1, ItemEntityID: itemID})
	steps(c, 6)
	if _, err := cfg.Apply(map[string]string{"zonelayer.normal_attack.cooldown_ticks": "5"}); err != nil { // a new version of the config: the engines record that they read it
		t.Fatal(err)
	}
	l.Send(command.ZoneLayerCharacterMoveCommand{CharacterID: 1, Dir: state.East, Ticks: 8})
	steps(c, 14)
	l.Send(command.ZoneLayerCharacterExitToCommand{CharacterID: 1, ExitEntityID: 1})
	steps(c, 60) // long enough for every engine to record a hash, in the zone it went to too

	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	compareWithFiles(t, dir, mem)
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
