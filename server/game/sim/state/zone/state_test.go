package zone_test

import (
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/zone"
	"github.com/huykingsofm/project-b/server/game/sim/state/zone/event"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap/tilemaptest"
	"github.com/huykingsofm/project-b/shared/table"
	"github.com/huykingsofm/project-b/shared/testing/aliasing"
)

// otherZone is the ID of another zone.
var otherZone = sim.NewStateID(state.ZoneType, 2)

// eastEdge is the last column of tiles of the map of newState, the whole height.
var eastEdge = tilemap.AreaOf(tilemap.Tile{X: 9, Y: 0}, tilemap.Tile{X: 9, Y: 4})

// newState is a zone on a map of 10 by 5 tiles that has an exit on its east edge.
func newState(t *testing.T) *zone.State {
	st := zone.NewState(9, tilemaptest.New(t, 10, 5, tilemap.Tile{}, nil))
	if _, ok := st.AddExit(eastEdge, otherZone, tilemap.Tile{X: 1, Y: 2}.Center()); !ok {
		t.Fatal("the exit was not added")
	}
	return st
}

func TestAnExitThatIsAddedStandsInTheZoneAsAnEntityAndTheClientsAreTold(t *testing.T) {
	st := newState(t)
	first := state.EntityID(1)
	exit, ok := st.ExitEntityByID(first)
	if !ok || exit.ToZoneID() != otherZone || exit.ID() != first || exit.Area() != eastEdge {
		t.Fatalf("ExitEntityByID(%d) = %+v, %v", first, exit, ok)
	}
	if _, ok := st.ExitEntityByID(state.EntityID(2)); ok {
		t.Fatal("found an exit that does not exist")
	}
	events := st.TakeEvents()
	want := event.ExitAdded{
		EntityID: first, Area: eastEdge, ToZoneID: otherZone,
		Arrival: tilemap.Tile{X: 1, Y: 2}.Center(),
	}
	if len(events) != 1 || events[0] != want {
		t.Fatalf("the clients are told %+v, want %+v", events, want)
	}
	second, ok := st.AddExit(eastEdge, otherZone, state.Vec{})
	if !ok || second != 2 {
		t.Fatalf("the second exit is %v, %v: the zone counts them", second, ok)
	}
}

func TestAnExitThatIsNotInTheMapOrNotToAZoneIsRefusedAndChangesNothing(t *testing.T) {
	st := newState(t)
	st.TakeEvents()
	before := st.Hash()
	inside := tilemap.AreaOf(tilemap.Tile{X: 1, Y: 1}, tilemap.Tile{X: 2, Y: 2})
	for name, c := range map[string]struct {
		area state.Rect
		to   sim.StateID
	}{
		"reversed": {state.Rect{Min: inside.Max, Max: inside.Min}, otherZone},
		"past the east edge": {
			tilemap.AreaOf(tilemap.Tile{X: 9, Y: 0}, tilemap.Tile{X: 10, Y: 4}), otherZone,
		},
		"before the map": {state.Rect{Min: state.Vec{X: -1}, Max: inside.Max}, otherZone},
		"to no zone":     {inside, 0},
		"to a world":     {inside, sim.NewStateID(state.WorldType, 1)},
	} {
		if id, ok := st.AddExit(c.area, c.to, state.Vec{}); ok || id != 0 {
			t.Errorf("%s: an exit was added (%v)", name, id)
		}
	}
	if st.Hash() != before || len(st.TakeEvents()) != 0 {
		t.Fatal("a refused exit changed the state or told the clients")
	}
}

// R13: nothing in a state is shared with its clone, except the map, which never changes.
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
	return reflect.TypeFor[table.Table[K, V]]().String() + ".cols"
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
