package testkit

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game"
	"github.com/huykingsofm/project-b/server/game/sim/command"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap/tilemaptest"
)

// The IDs of the states of the world that WorldBuilder makes. The builder gives the zonelayers the
// numbers 1, 2, 3 among the states of their type, in the order of their zones.
var (
	World  = sim.NewStateID(state.WorldType, 1)
	Zone1  = sim.NewStateID(state.ZoneType, 1)
	Zone2  = sim.NewStateID(state.ZoneType, 2)
	Layer1 = sim.NewStateID(state.ZoneLayerType, 1)
	Layer2 = sim.NewStateID(state.ZoneLayerType, 2)
	Layer3 = sim.NewStateID(state.ZoneLayerType, 3)
)

// EastExit is the ID of the exit on the east edge of Zone1, the first entity that the zone holds.
var EastExit = state.NewEntityID(Zone1, 1)

// WorldBuilder is the builder of the world of NewWorld: Zone1 (Layer1 and Layer2, two characters
// each) and Zone2 (Layer3, one character), each map 10 by 5 tiles, with an exit on the east edge
// of Zone1 to Zone2 and one on the west edge of Zone2 back to Zone1. A replay is given a builder
// like this one, for the cluster that was recorded.
func WorldBuilder(t testing.TB, seed uint64, regenEvery uint64) *game.ClusterBuilder {
	t.Helper()
	exit1 := tilemap.ExitDefinition{
		Min:      tilemap.Tile{X: 9, Y: 0},
		Max:      tilemap.Tile{X: 9, Y: 4},
		ToZoneID: Zone2,
		Arrival:  tilemap.Tile{X: 1, Y: 2},
	}
	exit2 := tilemap.ExitDefinition{
		Min:      tilemap.Tile{X: 0, Y: 0},
		Max:      tilemap.Tile{X: 0, Y: 4},
		ToZoneID: Zone1,
		Arrival:  tilemap.Tile{X: 8, Y: 2},
	}
	spawn := tilemap.Tile{X: 1, Y: 2}
	return game.NewBuilder().
		WithSeed(seed).
		WithWorld(game.WorldParams{ID: World}).
		WithZone(
			game.ZoneParams{
				ID:    Zone1,
				Tiles: tilemaptest.New(t, 10, 5, spawn, nil, []tilemap.ExitDefinition{exit1}),
			},
		).
		WithZoneLayers(
			game.ZoneLayerParams{ZoneID: Zone1, Count: 2, Capacity: 2, RegenEvery: regenEvery},
		).
		WithZone(
			game.ZoneParams{
				ID:    Zone2,
				Tiles: tilemaptest.New(t, 10, 5, spawn, nil, []tilemap.ExitDefinition{exit2}),
			},
		).
		WithZoneLayers(
			game.ZoneLayerParams{ZoneID: Zone2, Count: 1, Capacity: 1, RegenEvery: regenEvery},
		)
}

// NewWorld builds the world of WorldBuilder with a memory log and a Recorder of what the engines
// tell their clients.
func NewWorld(
	t testing.TB, seed uint64, regenEvery uint64,
) (*game.Cluster, *Recorder, *eventlog.MemLog) {
	t.Helper()
	rec, eventLog := &Recorder{}, &eventlog.MemLog{}
	c, err := WorldBuilder(t, seed, regenEvery).
		WithEventLog(eventLog).
		WithEmitter(rec).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	return c, rec, eventLog
}

// Steps runs n ticks of every engine of the cluster, and fails the test when one fails.
func Steps(t testing.TB, c *game.Cluster, n int) {
	t.Helper()
	for range n {
		if err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
}

func Enter(
	t *testing.T,
	c *game.Cluster,
	zoneID sim.StateID,
	characterID state.CharacterID,
	name string,
) {
	t.Helper()
	EnterCharacter(t, c, zoneID, NewCharacter(characterID, name, 100, 1000, 0))
}

// EnterCharacter puts a character, as it is, in the world through a zone.
func EnterCharacter(
	t *testing.T, c *game.Cluster, zoneID sim.StateID, character zonelayerstate.Character,
) {
	t.Helper()
	if !c.Zone(zoneID).Send(command.ZoneCharacterEnterCommand{Character: character}) {
		t.Fatal("zone queue full")
	}
	Steps(t, c, 4)
}

// WalkToTheEastExit enters a character in Zone1 and walks it to the exit.
func WalkToTheEastExit(t *testing.T, c *game.Cluster, characterID state.CharacterID) {
	t.Helper()
	Enter(t, c, Zone1, characterID, "An")
	c.ZoneLayer(Layer1).Send(
		command.ZoneLayerCharacterMoveCommand{CharacterID: characterID, Dir: state.East, Ticks: 8},
	)
	Steps(t, c, 12)
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
