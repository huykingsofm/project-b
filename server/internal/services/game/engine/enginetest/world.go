package enginetest

import (
	"testing"

	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/command"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap/tilemaptest"
)

// WorldBuilder is the builder of the world of NewWorld: zone 1 (zonelayers 1 and 2, two characters
// each) and zone 2 (zonelayer 3, one character), each map 10 by 5 tiles, with an exit on the east
// edge of zone 1 to zone 2 and one on the west edge of zone 2 back to zone 1. A replay is given a
// builder like this one, for the cluster that was recorded.
func WorldBuilder(t testing.TB, seed uint64, regenEvery uint64) *engine.ClusterBuilder {
	t.Helper()
	exit1 := tilemap.ExitDefinition{
		Min:      tilemap.Tile{X: 9, Y: 0},
		Max:      tilemap.Tile{X: 9, Y: 4},
		ToZoneID: 2,
		Arrival:  tilemap.Tile{X: 1, Y: 2},
	}
	exit2 := tilemap.ExitDefinition{
		Min:      tilemap.Tile{X: 0, Y: 0},
		Max:      tilemap.Tile{X: 0, Y: 4},
		ToZoneID: 1,
		Arrival:  tilemap.Tile{X: 8, Y: 2},
	}
	spawn := tilemap.Tile{X: 1, Y: 2}
	return engine.NewBuilder().
		WithSeed(seed).
		WithWorld(engine.WorldParams{ID: 1}).
		WithZone(
			engine.ZoneParams{
				ID:       1,
				Tiles:    tilemaptest.New(t, 10, 5, spawn, nil, []tilemap.ExitDefinition{exit1}),
				Adjacent: []state.ZoneID{2},
			},
		).
		WithZoneLayers(
			engine.ZoneLayerParams{ZoneID: 1, Count: 2, Capacity: 2, RegenEvery: regenEvery},
		).
		WithZone(
			engine.ZoneParams{
				ID:       2,
				Tiles:    tilemaptest.New(t, 10, 5, spawn, nil, []tilemap.ExitDefinition{exit2}),
				Adjacent: []state.ZoneID{1},
			},
		).
		WithZoneLayers(
			engine.ZoneLayerParams{ZoneID: 2, Count: 1, Capacity: 1, RegenEvery: regenEvery},
		)
}

// NewWorld builds the world of WorldBuilder with a memory log and a Recorder of what the engines
// tell their clients.
func NewWorld(
	t testing.TB, seed uint64, regenEvery uint64,
) (*engine.Cluster, *Recorder, *replay.MemLog, *config.Store) {
	t.Helper()
	rec, replayLog, cfg := &Recorder{}, &replay.MemLog{}, config.NewStore()
	c, err := WorldBuilder(t, seed, regenEvery).
		WithConfig(cfg).
		WithReplayLog(replayLog).
		WithWorldEmitter(rec).
		WithZoneEmitter(rec).
		WithZoneLayerEmitter(rec).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	return c, rec, replayLog, cfg
}

func Steps(c *engine.Cluster, n int) {
	for range n {
		c.Step()
	}
}

func Enter(
	t *testing.T,
	c *engine.Cluster,
	zoneID state.ZoneID,
	characterID state.CharacterID,
	name string,
) {
	t.Helper()
	EnterCharacter(t, c, zoneID, NewCharacter(characterID, name, 100, 1000, 0))
}

// EnterCharacter puts a character, as it is, in the world through a zone.
func EnterCharacter(
	t *testing.T, c *engine.Cluster, zoneID state.ZoneID, character zonelayerstate.Character,
) {
	t.Helper()
	if !c.Zone(zoneID).Send(command.ZoneCharacterEnterCommand{Character: character}) {
		t.Fatal("zone queue full")
	}
	Steps(c, 4)
}

// WalkToTheEastExit enters a character in zone 1 and walks it to the exit.
func WalkToTheEastExit(t *testing.T, c *engine.Cluster, characterID state.CharacterID) {
	t.Helper()
	Enter(t, c, 1, characterID, "An")
	c.ZoneLayer(1).Send(
		command.ZoneLayerCharacterMoveCommand{CharacterID: characterID, Dir: state.East, Ticks: 8},
	)
	Steps(c, 12)
}

// NewCharacter is a character of the values of a test, which are never invalid.
func NewCharacter(
	characterID state.CharacterID, name string, maxHealth, speed, radius int32,
) zonelayerstate.Character {
	c, err := zonelayerstate.NewCharacter(characterID, name, maxHealth, speed, radius)
	if err != nil {
		panic(err)
	}
	return c
}
