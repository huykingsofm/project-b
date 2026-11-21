package engine_test

import (
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/engine"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap"
)

func smallMap() *tilemap.TileMap {
	return tilemap.New(5, 5, state.Vec{X: 1, Y: 1}, nil, nil)
}

func TestABuilderWithOnlyAWorldBuildsAClusterWithTheDefaults(t *testing.T) {
	c, err := engine.NewBuilder().WithWorld(engine.WorldParams{ID: 1}).Build()
	if err != nil {
		t.Fatal(err)
	}
	c.Step() // the default emitters and replay log take what the engines give, and discard it
	if c.World == nil {
		t.Fatal("no world engine")
	}
}

func TestANilEmitterOrReplayLogKeepsTheDefault(t *testing.T) {
	c, err := engine.NewBuilder().
		WithReplayLog(nil).WithWorldEmitter(nil).WithZoneEmitter(nil).WithZoneLayerEmitter(nil).WithConfig(nil).
		WithWorld(engine.WorldParams{ID: 1}).
		WithZone(engine.ZoneParams{ID: 1, Tiles: smallMap()}).
		WithZoneLayers(engine.ZoneLayerParams{ZoneID: 1, Count: 1, Capacity: 2}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	c.Step()
}

func TestABuilderThatCannotMakeAClusterSaysWhy(t *testing.T) {
	world := engine.WorldParams{ID: 1}
	cases := map[string]*engine.ClusterBuilder{
		"no world":                                engine.NewBuilder(),
		"a queue of nothing":                      engine.NewBuilder().WithWorld(world).WithQueueSize(0),
		"a zone without a map":                    engine.NewBuilder().WithWorld(world).WithZone(engine.ZoneParams{ID: 1}),
		"the same zone twice":                     engine.NewBuilder().WithWorld(world).WithZone(engine.ZoneParams{ID: 1, Tiles: smallMap()}).WithZone(engine.ZoneParams{ID: 1, Tiles: smallMap()}),
		"zonelayers of a zone that was not added": engine.NewBuilder().WithWorld(world).WithZoneLayers(engine.ZoneLayerParams{ZoneID: 9, Count: 1, Capacity: 1}),
		"zonelayers set twice": engine.NewBuilder().WithWorld(world).WithZone(engine.ZoneParams{ID: 1, Tiles: smallMap()}).
			WithZoneLayers(engine.ZoneLayerParams{ZoneID: 1, Count: 1, Capacity: 1}).
			WithZoneLayers(engine.ZoneLayerParams{ZoneID: 1, Count: 1, Capacity: 1}),
		"zonelayers that hold nobody": engine.NewBuilder().WithWorld(world).WithZone(engine.ZoneParams{ID: 1, Tiles: smallMap()}).
			WithZoneLayers(engine.ZoneLayerParams{ZoneID: 1, Count: 1, Capacity: 0}),
	}
	for name, b := range cases {
		if c, err := b.Build(); err == nil || c != nil {
			t.Errorf("%s: built a cluster, want an error", name)
		}
	}
}
