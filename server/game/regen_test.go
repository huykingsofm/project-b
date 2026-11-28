package game_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap/tilemaptest"
	"github.com/huykingsofm/project-b/server/game/testkit"
)

func TestAZonelayerThatTheWorldAskedForHealsAtThePeriodAskedAndOneAskedForNoHealWaitsForNone(
	t *testing.T,
) {
	c := testkit.Make(t, testkit.Options(t, 7), nil, testkit.ZoneSpec{
		Tiles:  tilemaptest.New(t, 10, 5, tilemap.Tile{}, nil),
		Layers: []testkit.LayerSpec{{Capacity: 1, RegenEvery: 10}, {Capacity: 1}},
	})
	heals, ok := c.ZoneLayer(testkit.Layer1).Scheduler().FindByID(
		action.ZoneLayerCharacterRegenActionID,
	)
	if !ok || heals.(action.ZoneLayerCharacterRegenAction).Every != 10 {
		t.Fatalf("the zonelayer does not wait for the heal that was asked for: %v, %v", heals, ok)
	}
	if _, waits := c.ZoneLayer(testkit.Layer2).Scheduler().FindByID(
		action.ZoneLayerCharacterRegenActionID,
	); waits {
		t.Fatal("a zonelayer that heals every 0 ticks waits for a heal")
	}
}

func TestAZonelayerMadeAgainFromItsStartHealsAtTheSamePeriod(t *testing.T) {
	c, _, mem := testkit.NewWorld(t, 7, 10)
	testkit.Steps(t, c, 2)
	started := mem.ByEngine()[testkit.Layer1][0].Body.(eventlog.Init)
	made, err := c.Env.EngineRegistry.Restore(aFreshEnv(c), started.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	layer := made.(*engine.Engine[*zonelayerstate.State])
	heals, ok := layer.Scheduler().FindByID(action.ZoneLayerCharacterRegenActionID)
	if !ok || heals.(action.ZoneLayerCharacterRegenAction).Every != 10 {
		t.Fatalf("the engine that was made from its start does not wait for the heal: %v, %v",
			heals, ok)
	}
	if layer.Tick() != 0 {
		t.Fatalf("the engine made from its start is at the tick %d, want 0", layer.Tick())
	}
}
