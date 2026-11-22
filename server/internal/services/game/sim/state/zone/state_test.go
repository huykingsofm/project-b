package zone_test

import (
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state/entity"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap/tilemaptest"
	"github.com/huykingsofm/project-b/server/internal/shared/testing/aliasing"
)

func newState(t *testing.T) *zone.State {
	exit := tilemap.ExitDefinition{Min: tilemap.Tile{X: 9, Y: 0}, Max: tilemap.Tile{X: 9, Y: 4}, ToZoneID: 2}
	return zone.NewState(1, tilemaptest.New(t, 10, 5, tilemap.Tile{}, nil, []tilemap.ExitDefinition{exit}))
}

func TestTheExitsOfTheMapStandInTheZoneAsEntities(t *testing.T) {
	st := newState(t)
	exit, ok := st.ExitEntityByID(1)
	if !ok || exit.ToZoneID != 2 || exit.EntityID != 1 {
		t.Fatalf("ExitEntityByID(1) = %+v, %v", exit, ok)
	}
	if _, ok := st.ExitEntityByID(2); ok {
		t.Fatal("found an exit that does not exist")
	}
}

// R29: nothing in a state is shared with its clone, except the map, which never changes.
func TestAChangeAfterCloneDoesNotChangeTheCopyAndTheOtherWayRound(t *testing.T) {
	st := newState(t)
	st.ZoneLayers.Set(1, zone.ZoneLayerSlot{ID: 1, Capacity: 4})
	before := st.Hash()
	copied := st.Clone()

	st.ZoneLayers.Set(2, zone.ZoneLayerSlot{ID: 2, Capacity: 4})
	st.ZoneLayers.Set(1, zone.ZoneLayerSlot{ID: 1, Capacity: 9})
	st.Entities.Delete(1)
	if copied.Hash() != before || copied.ZoneLayers.Len() != 1 || copied.Entities.Len() != 1 {
		t.Fatal("a change of the state changed its copy")
	}
	copied.ZoneLayers.Delete(1)
	if st.ZoneLayers.Len() != 2 {
		t.Fatal("a change of the copy changed the state")
	}
}

func TestAClonedZoneStateSharesNoMemoryWithTheStateExceptTheMapThatNeverChanges(t *testing.T) {
	st := newState(t)
	st.ZoneLayers.Set(1, zone.ZoneLayerSlot{ID: 1, Capacity: 4})
	copied := st.Clone()
	if copied.Tiles != st.Tiles {
		t.Fatal("the map of the zone is meant to be shared")
	}
	// the map is shared on purpose, so it is not looked at: compare the rest
	a, b := *st, *copied
	a.Tiles, b.Tiles = nil, nil
	if err := aliasing.Disjoint(a, b); err != nil {
		t.Fatal(err)
	}
}

func TestTheTypesOfTheZoneStateObeyTheRuleOfData(t *testing.T) {
	rules := aliasing.Rules{
		Interfaces: []reflect.Type{reflect.TypeFor[entity.Entity]()},
		Shared:     []reflect.Type{reflect.TypeFor[*tilemap.TileMap]()},
		Skip:       []string{"zone.State.events"},
	}
	if err := aliasing.CheckCloneable(reflect.TypeFor[zone.State](), rules); err != nil {
		t.Fatal(err)
	}
}
