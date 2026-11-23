package world_test

import (
	"bytes"
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state/world"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

func TestAWorldStateComesBackAsItWasWrittenAndCutBytesAreAnError(t *testing.T) {
	st := world.NewState(1)
	st.AddItemToCharacter(3, 7, 120, 4, 99)
	st.AddItemToCharacter(4, 8, 5, 4, 99)
	st.SendPrivateMessage(world.NewPrivateChatMessage(1, 2, "psst", 5))
	st.SetTick(9)
	st.AddZone(2)
	st.AddZone(3)
	st.AddZone(2) // a zone it knows is not added again
	st.AddZoneLayer(4)
	st.AddZoneLayer(5)
	st.RemoveZoneLayer(4)

	var w binary.Writer
	st.Encode(&w)
	r := binary.NewReader(w.B)
	got, err := world.DecodeState(r, 1)
	if err != nil || r.Left() != 0 {
		t.Fatalf("error %v, %d bytes left", err, r.Left())
	}
	zones, zoneLayers := []sim.StateID{2, 3}, []sim.StateID{5}
	if !slices.Equal(got.ZoneIDs(), zones) || !slices.Equal(got.ZoneLayerIDs(), zoneLayers) {
		t.Fatalf("the world knows zones %v and zonelayers %v", got.ZoneIDs(), got.ZoneLayerIDs())
	}
	if got.Hash() != st.Hash() {
		t.Fatal("the state that came back has another hash")
	}
	var again binary.Writer
	got.Encode(&again)
	if !bytes.Equal(again.B, w.B) {
		t.Fatal("the state that came back is written differently")
	}
	for n := range w.B { // every proper prefix
		if _, err := world.DecodeState(binary.NewReader(w.B[:n]), 1); err == nil {
			t.Fatalf("%d of %d bytes were read", n, len(w.B))
		}
	}
}
