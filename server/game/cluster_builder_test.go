package game_test

import (
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap/tilemaptest"
)

func smallMap(t testing.TB) *tilemap.TileMap {
	return tilemaptest.New(t, 5, 5, tilemap.Tile{X: 1, Y: 1}, nil, nil)
}

func TestABuilderWithOnlyAWorldBuildsAClusterWithTheDefaults(t *testing.T) {
	c, err := game.NewBuilder().WithWorld(game.WorldParams{ID: 1}).Build()
	if err != nil {
		t.Fatal(err)
	}
	c.Step() // the default emitters and replay log take what the engines give, and discard it
	if c.World == nil {
		t.Fatal("no world engine")
	}
}

func TestANilEmitterOrReplayLogKeepsTheDefault(t *testing.T) {
	c, err := game.NewBuilder().
		WithReplayLog(nil).WithEmitter(nil).WithConfig(nil).
		WithWorld(game.WorldParams{ID: 1}).
		WithZone(game.ZoneParams{ID: 2, Tiles: smallMap(t)}).
		WithZoneLayers(game.ZoneLayerParams{ZoneID: 2, Count: 1, Capacity: 2}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	c.Step()
}

func TestAWorldAndAZoneThatShareAnIDAreRefusedBecauseEveryStateHasAnIDOfItsOwn(t *testing.T) {
	_, err := game.NewBuilder().
		WithWorld(game.WorldParams{ID: 1}).
		WithZone(game.ZoneParams{ID: 1, Tiles: smallMap(t)}).
		Build()
	if err == nil {
		t.Fatal("a zone with the ID of the world was built")
	}
}

func TestTheZoneLayersTakeTheIDsAfterTheLargestOneGivenAndEveryEngineKnowsTheWorld(t *testing.T) {
	c, err := game.NewBuilder().
		WithWorld(game.WorldParams{ID: 1}).
		WithZone(game.ZoneParams{ID: 5, Tiles: smallMap(t), Adjacent: []sim.StateID{3}}).
		WithZone(game.ZoneParams{ID: 3, Tiles: smallMap(t)}).
		WithZoneLayers(game.ZoneLayerParams{ZoneID: 5, Count: 2, Capacity: 1}).
		WithZoneLayers(game.ZoneLayerParams{ZoneID: 3, Count: 1, Capacity: 1}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	var layers []sim.StateID
	for _, l := range c.ZoneLayers() {
		layers = append(layers, l.StateID())
		if l.State().WorldID != 1 {
			t.Fatalf("zonelayer %d does not know the world", l.StateID())
		}
	}
	if !slices.Equal(layers, []sim.StateID{6, 7, 8}) {
		t.Fatalf("zonelayers %v, want 6, 7 and 8", layers)
	}
	world := c.World.State()
	if !slices.Equal(world.ZoneIDs(), []sim.StateID{5, 3}) ||
		!slices.Equal(world.ZoneLayerIDs(), layers) {
		t.Fatalf(
			"the world knows zones %v and zonelayers %v", world.ZoneIDs(), world.ZoneLayerIDs(),
		)
	}
	if z := c.Zone(5).State(); z.WorldID != 1 || !slices.Equal(z.Adjacent, []sim.StateID{3}) ||
		z.ZoneLayers.Len() != 2 {
		t.Fatalf(
			"zone 5 knows world %d, adjacent %v, %d zonelayers",
			z.WorldID, z.Adjacent, z.ZoneLayers.Len(),
		)
	}
	if c.ZoneLayer(6).State().ZoneID != 5 || c.ZoneLayer(8).State().ZoneID != 3 {
		t.Fatal("a zonelayer does not know its zone")
	}
}

func TestABuilderThatCannotMakeAClusterSaysWhy(t *testing.T) {
	world := game.WorldParams{ID: 1}
	cases := map[string]*game.ClusterBuilder{
		"no world": game.NewBuilder(),
		"a queue of nothing": game.NewBuilder().WithWorld(
			world,
		).WithQueueSize(0),
		"a zone without a map": game.NewBuilder().WithWorld(world).WithZone(
			game.ZoneParams{ID: 2},
		),
		"the same zone twice": game.NewBuilder().WithWorld(world).WithZone(
			game.ZoneParams{ID: 2, Tiles: smallMap(t)},
		).WithZone(game.ZoneParams{ID: 2, Tiles: smallMap(t)}),
		"zonelayers of a zone that was not added": game.NewBuilder().WithWorld(
			world,
		).WithZoneLayers(game.ZoneLayerParams{ZoneID: 9, Count: 1, Capacity: 1}),
		"zonelayers set twice": game.NewBuilder().WithWorld(world).WithZone(
			game.ZoneParams{ID: 2, Tiles: smallMap(t)},
		).
			WithZoneLayers(game.ZoneLayerParams{ZoneID: 2, Count: 1, Capacity: 1}).
			WithZoneLayers(game.ZoneLayerParams{ZoneID: 2, Count: 1, Capacity: 1}),
		"zonelayers that hold nobody": game.NewBuilder().WithWorld(world).WithZone(
			game.ZoneParams{ID: 2, Tiles: smallMap(t)},
		).
			WithZoneLayers(game.ZoneLayerParams{ZoneID: 2, Count: 1, Capacity: 0}),
	}
	for name, b := range cases {
		if c, err := b.Build(); err == nil || c != nil {
			t.Errorf("%s: built a cluster, want an error", name)
		}
	}
}
