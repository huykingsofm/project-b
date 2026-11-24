package game_test

import (
	"testing"

	"github.com/huykingsofm/project-b/server/game"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap/tilemaptest"
	"github.com/huykingsofm/project-b/server/game/testkit"
)

func TestEnsureRegenSetsTheFirstActionWhenNoneWaitsAndChecksTheOneThatDoes(t *testing.T) {
	c := aWorldToCheckpoint(t) // the builder asked for healing every 10 ticks
	l := c.ZoneLayer(theLayer)
	regen, ok := l.Scheduler().FindByID(action.ZoneLayerCharacterRegenActionID)
	if !ok || regen.(action.ZoneLayerCharacterRegenAction).Every != 10 {
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
	data, err := l.Checkpoint().Encode()
	if err != nil {
		t.Fatal(err)
	}
	r, err := restoreEngine[*zonelayerstate.State](data, c.Env, 7)
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
