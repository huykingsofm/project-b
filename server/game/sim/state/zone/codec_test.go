package zone_test

import (
	"bytes"
	"testing"

	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/zone"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

func TestAZoneStateComesBackAsItWasWrittenWithItsMap(t *testing.T) {
	st := newState(t)
	st.AddZoneLayer(4, 50)
	st.AddZoneLayer(5, 20)
	st.SendMessage(zone.NewChatMessage(1, "hello zone", 3))
	for range 7 {
		st.AdvanceTick()
	}

	var w binary.Writer
	if err := st.Encode(&w); err != nil {
		t.Fatal(err)
	}
	r := binary.NewReader(w.B)
	got, err := zone.DecodeState(r, 1)
	if err != nil || r.Left() != 0 {
		t.Fatalf("error %v, %d bytes left", err, r.Left())
	}
	if got.Hash() != st.Hash() {
		t.Fatal("the state that came back has another hash")
	}
	if got.WorldID() != st.WorldID() || got.WorldID() == 0 {
		t.Fatalf("the zone knows world %d", got.WorldID())
	}
	if exit, ok := got.ExitEntityByID(1); !ok || exit.Area() != eastEdge {
		t.Fatalf("the zone came back with the exit %+v, %v", exit, ok)
	}
	if got.TileMap().Free(state.Vec{X: -1}) {
		t.Fatal("the map came back wrong")
	}
	var again binary.Writer
	if err := got.Encode(&again); err != nil || !bytes.Equal(again.B, w.B) {
		t.Fatalf("the state that came back is written differently: %v", err)
	}
	for n := range w.B { // every proper prefix
		if _, err := zone.DecodeState(binary.NewReader(w.B[:n]), 1); err == nil {
			t.Fatalf("%d of %d bytes were read", n, len(w.B))
		}
	}
}
