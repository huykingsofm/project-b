package zonelayer

import (
	"math"
	"testing"

	"github.com/huykingsofm/project-b/server/game/sim/state"
)

func TestAnEntityIsNotAddedWhenTheZoneLayerHasNoEntityIDLeft(t *testing.T) {
	st := NewState(1, 1, 9)
	at := state.Vec{X: 3500, Y: 3500}
	st.lastEntityID = math.MaxUint64 - 1
	id, err := st.AddItemEntity(7, 1, at, 0)
	if err != nil || id.Local != math.MaxUint64 {
		t.Fatalf("the last number was refused: %v, %v", id, err)
	}
	if _, err := st.AddItemEntity(7, 1, at, 0); err == nil {
		t.Fatal("an item was added after the last number was given")
	}
	if got := len(st.EntityIDs()); got != 1 {
		t.Fatalf("%d entities after a refusal, want 1", got)
	}
}
