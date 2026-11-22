package state_test

import (
	"testing"

	"github.com/huykingsofm/project-b/server/game/sim/state"
)

func TestAnAudienceKeepsNothingOfTheSliceItWasMadeFrom(t *testing.T) {
	ids := []state.CharacterID{1, 2}
	a := state.AudienceCharacters(ids...)
	ids[0] = 99
	if a.Characters[0] != 1 {
		t.Fatal("changing the slice that built the audience changed the audience")
	}
}

func TestRectCoveringHoldsBothPositionsInAnyOrder(t *testing.T) {
	a, b := state.Vec{X: 5, Y: -2}, state.Vec{X: -1, Y: 7}
	want := state.Rect{Min: state.Vec{X: -1, Y: -2}, Max: state.Vec{X: 5, Y: 7}}
	if state.RectCovering(a, b) != want || state.RectCovering(b, a) != want {
		t.Fatalf("RectCovering = %+v, want %+v", state.RectCovering(a, b), want)
	}
}
