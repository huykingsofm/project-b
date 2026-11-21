package tilemap_test

import (
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap"
)

func TestFreeTilesAreInsideTheMapAndNotWalls(t *testing.T) {
	m := tilemap.New(4, 3, state.Vec{X: 1, Y: 1}, []state.Vec{{X: 2, Y: 1}}, nil)
	for _, c := range []struct {
		at   state.Vec
		want bool
	}{
		{state.Vec{X: 0, Y: 0}, true},
		{state.Vec{X: 3, Y: 2}, true},
		{state.Vec{X: 2, Y: 1}, false},  // a wall
		{state.Vec{X: -1, Y: 0}, false}, // outside
		{state.Vec{X: 4, Y: 0}, false},
		{state.Vec{X: 0, Y: 3}, false},
	} {
		if got := m.Free(c.at); got != c.want {
			t.Errorf("Free(%+v) = %v, want %v", c.at, got, c.want)
		}
	}
}

func TestExitsAreFoundByPositionAndByID(t *testing.T) {
	exit := tilemap.Exit{ID: 7, Min: state.Vec{X: 3, Y: 0}, Max: state.Vec{X: 3, Y: 2}, ToZoneID: 2, Arrival: state.Vec{X: 1, Y: 1}}
	m := tilemap.New(4, 3, state.Vec{}, nil, []tilemap.Exit{exit})

	if got, ok := m.ExitAt(state.Vec{X: 3, Y: 1}); !ok || got.ID != 7 {
		t.Fatalf("ExitAt inside = %+v, %v", got, ok)
	}
	if _, ok := m.ExitAt(state.Vec{X: 2, Y: 1}); ok {
		t.Fatal("ExitAt outside found an exit")
	}
	if got, ok := m.ExitByID(7); !ok || got.ToZoneID != 2 {
		t.Fatalf("ExitByID = %+v, %v", got, ok)
	}
	if _, ok := m.ExitByID(8); ok {
		t.Fatal("ExitByID found an exit that does not exist")
	}
}
