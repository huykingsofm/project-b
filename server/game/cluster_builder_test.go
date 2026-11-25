package game_test

import (
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap/tilemaptest"
)

func worldID(n uint64) sim.StateID { return sim.NewStateID(state.WorldType, n) }
func zoneID(n uint64) sim.StateID  { return sim.NewStateID(state.ZoneType, n) }
func layerID(n uint64) sim.StateID { return sim.NewStateID(state.ZoneLayerType, n) }

func smallMap(t testing.TB) *tilemap.TileMap {
	return tilemaptest.New(t, 5, 5, tilemap.Tile{X: 1, Y: 1}, nil, nil)
}

func TestABuilderWithOnlyAWorldBuildsAClusterWithTheDefaults(t *testing.T) {
	c, err := game.NewBuilder().WithWorld(game.WorldParams{ID: worldID(1)}).Build()
	if err != nil {
		t.Fatal(err)
	}
	c.Step() // the default emitters and event log take what the engines give, and discard it
	if c.World == nil {
		t.Fatal("no world engine")
	}
}

func TestANilEmitterOrEventLogKeepsTheDefault(t *testing.T) {
	c, err := game.NewBuilder().
		WithEventLog(nil).WithEmitter(nil).
		WithWorld(game.WorldParams{ID: worldID(1)}).
		WithZone(game.ZoneParams{ID: zoneID(2), Tiles: smallMap(t)}).
		WithZoneLayers(game.ZoneLayerParams{ZoneID: zoneID(2), Count: 1, Capacity: 2}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	c.Step()
}

func TestAnIDThatIsNotOfTheTypeOfItsStateIsRefused(t *testing.T) {
	_, err := game.NewBuilder().
		WithWorld(game.WorldParams{ID: worldID(1)}).
		WithZone(game.ZoneParams{ID: worldID(2), Tiles: smallMap(t)}).
		Build()
	if err == nil {
		t.Fatal("a zone with the ID of a world was built")
	}
	_, err = game.NewBuilder().WithWorld(game.WorldParams{ID: zoneID(1)}).Build()
	if err == nil {
		t.Fatal("a world with the ID of a zone was built")
	}
}

func TestTheZoneLayersAreNumberedFromOneAndEveryEngineKnowsTheWorld(t *testing.T) {
	c, err := game.NewBuilder().
		WithWorld(game.WorldParams{ID: worldID(1)}).
		WithZone(game.ZoneParams{ID: zoneID(5), Tiles: smallMap(t)}).
		WithZone(game.ZoneParams{ID: zoneID(3), Tiles: smallMap(t)}).
		WithZoneLayers(game.ZoneLayerParams{ZoneID: zoneID(5), Count: 2, Capacity: 1}).
		WithZoneLayers(game.ZoneLayerParams{ZoneID: zoneID(3), Count: 1, Capacity: 1}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	var layers []sim.StateID
	for _, l := range c.ZoneLayers() {
		layers = append(layers, l.StateID())
		if l.State().WorldID() != worldID(1) {
			t.Fatalf("zonelayer %d does not know the world", l.StateID())
		}
	}
	if !slices.Equal(layers, []sim.StateID{layerID(1), layerID(2), layerID(3)}) {
		t.Fatalf("zonelayers %v, want 6, 7 and 8", layers)
	}
	world := c.World.State()
	if !slices.Equal(world.ZoneIDs(), []sim.StateID{zoneID(5), zoneID(3)}) ||
		!slices.Equal(world.ZoneLayerIDs(), layers) {
		t.Fatalf(
			"the world knows zones %v and zonelayers %v", world.ZoneIDs(), world.ZoneLayerIDs(),
		)
	}
	z := c.Zone(zoneID(5)).State()
	if z.WorldID() != worldID(1) || z.ZoneLayerCount() != 2 {
		t.Fatalf("zone 5 knows world %d and %d zonelayers", z.WorldID(), z.ZoneLayerCount())
	}
	if c.ZoneLayer(layerID(1)).State().ZoneID() != zoneID(5) ||
		c.ZoneLayer(layerID(3)).State().ZoneID() != zoneID(3) {
		t.Fatal("a zonelayer does not know its zone")
	}
}

func TestABuilderThatCannotMakeAClusterSaysWhy(t *testing.T) {
	world := game.WorldParams{ID: worldID(1)}
	cases := map[string]*game.ClusterBuilder{
		"no world": game.NewBuilder(),
		"a queue of nothing": game.NewBuilder().WithWorld(
			world,
		).WithQueueSize(0),
		"a zone without a map": game.NewBuilder().WithWorld(world).WithZone(
			game.ZoneParams{ID: zoneID(2)},
		),
		"the same zone twice": game.NewBuilder().WithWorld(world).WithZone(
			game.ZoneParams{ID: zoneID(2), Tiles: smallMap(t)},
		).WithZone(game.ZoneParams{ID: zoneID(2), Tiles: smallMap(t)}),
		"zonelayers of a zone that was not added": game.NewBuilder().WithWorld(
			world,
		).WithZoneLayers(game.ZoneLayerParams{ZoneID: zoneID(9), Count: 1, Capacity: 1}),
		"zonelayers set twice": game.NewBuilder().WithWorld(world).WithZone(
			game.ZoneParams{ID: zoneID(2), Tiles: smallMap(t)},
		).
			WithZoneLayers(game.ZoneLayerParams{ZoneID: zoneID(2), Count: 1, Capacity: 1}).
			WithZoneLayers(game.ZoneLayerParams{ZoneID: zoneID(2), Count: 1, Capacity: 1}),
		"zonelayers that hold nobody": game.NewBuilder().WithWorld(world).WithZone(
			game.ZoneParams{ID: zoneID(2), Tiles: smallMap(t)},
		).
			WithZoneLayers(game.ZoneLayerParams{ZoneID: zoneID(2), Count: 1, Capacity: 0}),
	}
	for name, b := range cases {
		if c, err := b.Build(); err == nil || c != nil {
			t.Errorf("%s: built a cluster, want an error", name)
		}
	}
}
