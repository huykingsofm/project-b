package zone_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/zone"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap/tilemaptest"
)

// twoZone is the ID of the second zone: the type of a zone and the number 2.
var twoZone = sim.NewStateID(state.ZoneType, 2)

func TestAClonedZoneStateSharesNoMemoryWithTheStateExceptTheMapThatNeverChanges(t *testing.T) {
	st := zone.NewState(9, tilemaptest.New(t, 10, 5, tilemap.Tile{}, nil))
	east := tilemap.AreaOf(tilemap.Tile{X: 9, Y: 0}, tilemap.Tile{X: 9, Y: 4})
	st.AddExit(east, twoZone, state.Vec{})
	st.AddZoneLayer(1, 4)
	copied := st.Clone().(*zone.State)
	if copied.TileMap() != st.TileMap() {
		t.Fatal("the map of the zone is meant to be shared")
	}
	// the rest is the copy's own: what is added to the copy is not in the state
	copied.AddZoneLayer(2, 4)
	copied.AddExit(east, twoZone, state.Vec{})
	if st.ZoneLayerCount() != 1 || st.ExitEntityCount() != 1 {
		t.Fatalf("the state has %d zonelayers and %d exits after the copy got more, want 1+1",
			st.ZoneLayerCount(), st.ExitEntityCount())
	}
}
