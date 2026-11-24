package zone_test

import (
	"bytes"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state/zone"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

func TestAZoneStateComesBackAsItWasWrittenWithTheMapThatItsIDFinds(t *testing.T) {
	st := newState(t)
	st.AddZoneLayer(4, 50)
	st.AddZoneLayer(5, 20)
	st.SendMessage(zone.NewChatMessage(1, "hello zone", 3))
	for range 7 {
		st.NextTick()
	}
	tiles := st.TileMap().(*tilemap.TileMap)

	var w binary.Writer
	if err := st.Encode(&w); err != nil {
		t.Fatal(err)
	}
	tilesOf := func(id sim.StateID) *tilemap.TileMap {
		if id == 1 {
			return tiles
		}
		return nil
	}
	r := binary.NewReader(w.B)
	got, err := zone.DecodeState(r, 1, tilesOf)
	if err != nil || r.Left() != 0 {
		t.Fatalf("error %v, %d bytes left", err, r.Left())
	}
	if got.Hash() != st.Hash() || got.TileMap() != tiles {
		t.Fatal("the state that came back has another hash, or another map")
	}
	if got.WorldID() != st.WorldID() || got.WorldID() == 0 {
		t.Fatalf("the zone knows world %d", got.WorldID())
	}
	var again binary.Writer
	if err := got.Encode(&again); err != nil || !bytes.Equal(again.B, w.B) {
		t.Fatalf("the state that came back is written differently: %v", err)
	}
	for n := range w.B { // every proper prefix
		if _, err := zone.DecodeState(binary.NewReader(w.B[:n]), 1, tilesOf); err == nil {
			t.Fatalf("%d of %d bytes were read", n, len(w.B))
		}
	}
	none := func(sim.StateID) *tilemap.TileMap { return nil }
	if _, err := zone.DecodeState(binary.NewReader(w.B), 1, none); err == nil {
		t.Fatal("a zone with no map was read")
	}
}
