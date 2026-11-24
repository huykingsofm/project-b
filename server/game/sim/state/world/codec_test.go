package world_test

import (
	"bytes"
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/world"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

func TestAWorldStateComesBackAsItWasWrittenAndCutBytesAreAnError(t *testing.T) {
	st := world.NewState(1)
	st.AddItemToCharacter(3, 7, 120, 4, 99)
	st.AddItemToCharacter(4, 8, 5, 4, 99)
	st.SendPrivateMessage(world.NewPrivateChatMessage(1, 2, "psst", 5))
	for range 9 {
		st.NextTick()
	}
	st.AddZone(2)
	st.AddZone(3)
	st.AddZone(2) // a zone it knows is not added again
	first := st.AddZoneLayer()
	st.AddZoneLayer()
	st.RemoveZoneLayer(first)

	var w binary.Writer
	st.Encode(&w)
	r := binary.NewReader(w.B)
	got, err := world.DecodeState(r, 1)
	if err != nil || r.Left() != 0 {
		t.Fatalf("error %v, %d bytes left", err, r.Left())
	}
	zones := []sim.StateID{2, 3}
	zoneLayers := []sim.StateID{sim.NewStateID(state.ZoneLayerType, 2)}
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

// worldBytes writes a world that knows the zonelayers (no zone) and gave the numbers up to last.
func worldBytes(zoneLayers []sim.StateID, last uint64) []byte {
	var w binary.Writer
	w.Uvarint(1) // the ID of the world
	w.Uvarint(0) // its tick
	w.Uvarint(0) // no zone
	w.Uvarint(uint64(len(zoneLayers)))
	for _, id := range zoneLayers {
		w.Uvarint(uint64(id))
	}
	w.Uvarint(last) // the number that the world gave last
	w.Uvarint(0)    // no private message
	w.Uvarint(0)    // no inventory
	return w.B
}

func TestAWorldWhoseZoneLayersWereNotGivenUpToItsLastNumberIsAnError(t *testing.T) {
	layer := func(n uint64) sim.StateID { return sim.NewStateID(state.ZoneLayerType, n) }
	aZone := sim.NewStateID(state.ZoneType, 1)
	given := worldBytes([]sim.StateID{layer(1), layer(3)}, 3)
	if _, err := world.DecodeState(binary.NewReader(given), 1); err != nil {
		t.Fatalf("zonelayers that were given: %v", err)
	}
	for name, b := range map[string][]byte{
		"a number that was never given": worldBytes([]sim.StateID{layer(4)}, 3),
		"the number 0":                  worldBytes([]sim.StateID{layer(0)}, 3),
		"a zonelayer twice":             worldBytes([]sim.StateID{layer(2), layer(2)}, 3),
		"a state of another type":       worldBytes([]sim.StateID{aZone}, 3),
	} {
		if _, err := world.DecodeState(binary.NewReader(b), 1); err == nil {
			t.Errorf("a world with %s was read", name)
		}
	}
}
