package zone

import (
	"testing"

	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap/tilemaptest"
	"github.com/huykingsofm/project-b/shared/testing/aliasing"
)

func TestAClonedZoneStateSharesNoMemoryWithTheStateExceptTheMapThatNeverChanges(t *testing.T) {
	exit := tilemap.ExitDefinition{
		Min: tilemap.Tile{X: 9, Y: 0}, Max: tilemap.Tile{X: 9, Y: 4}, ToZoneID: 2,
	}
	st := NewState(
		1, 9, 1000,
		tilemaptest.New(t, 10, 5, tilemap.Tile{}, nil, []tilemap.ExitDefinition{exit}),
	)
	st.AddZoneLayer(1, 4)
	copied := st.Clone().(*State)
	if copied.tiles != st.tiles {
		t.Fatal("the map of the zone is meant to be shared")
	}
	// the map is shared on purpose, so it is not looked at: compare the rest
	a, b := *st, *copied
	a.tiles, b.tiles = nil, nil
	if err := aliasing.Disjoint(a, b); err != nil {
		t.Fatal(err)
	}
}
