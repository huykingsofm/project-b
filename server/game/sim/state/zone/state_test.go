package zone_test

import (
	"github.com/huykingsofm/project-b/shared/table"
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/zone"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap/tilemaptest"
	"github.com/huykingsofm/project-b/shared/testing/aliasing"
)

func newState(t *testing.T) *zone.State {
	exit := tilemap.ExitDefinition{
		Min: tilemap.Tile{X: 9, Y: 0}, Max: tilemap.Tile{X: 9, Y: 4}, ToZoneID: 2,
	}
	return zone.NewState(
		1, 9, tilemaptest.New(t, 10, 5, tilemap.Tile{}, nil, []tilemap.ExitDefinition{exit}), 2, 3,
	)
}

func TestTheExitsOfTheMapStandInTheZoneAsEntities(t *testing.T) {
	st := newState(t)
	first := state.NewEntityID(1, 1)
	exit, ok := st.ExitEntityByID(first)
	if !ok || exit.ToZoneID() != 2 || exit.ID() != first || first.Owner() != 1 {
		t.Fatalf("ExitEntityByID(%d) = %+v, %v", first, exit, ok)
	}
	if _, ok := st.ExitEntityByID(state.NewEntityID(1, 2)); ok {
		t.Fatal("found an exit that does not exist")
	}
}

// R29: nothing in a state is shared with its clone, except the map, which never changes.
func TestAChangeAfterCloneDoesNotChangeTheCopyAndTheOtherWayRound(t *testing.T) {
	st := newState(t)
	st.AddZoneLayer(1, 4)
	before := st.Hash()
	copied := st.Clone().(*zone.State)

	st.AddZoneLayer(2, 4)
	st.AddZoneLayer(1, 9)
	if copied.Hash() != before || copied.ZoneLayerCount() != 1 || copied.ExitEntityCount() != 1 {
		t.Fatal("a change of the state changed its copy")
	}
	copied.RemoveZoneLayer(1)
	if st.ZoneLayerCount() != 2 {
		t.Fatal("a change of the copy changed the state")
	}
}

// columnsOf names the field of the columns of a table, as the rules name a field to leave out.
func columnsOf[K comparable, V any]() string {
	return reflect.TypeFor[table.Table[K, V]]().String() + ".columns"
}

func TestTheTypesOfTheZoneStateObeyTheRuleOfData(t *testing.T) {
	rules := aliasing.Rules{
		Interfaces: []reflect.Type{reflect.TypeFor[state.Entity]()},
		Shared:     []reflect.Type{reflect.TypeFor[*tilemap.TileMap]()},
		// the columns of a table are copied by its Clone, and Disjoint checks that on a real value
		Skip: []string{"zone.State.events", columnsOf[state.EntityID, state.Entity]()},
	}
	if err := aliasing.CheckCloneable(reflect.TypeFor[zone.State](), rules); err != nil {
		t.Fatal(err)
	}
}
