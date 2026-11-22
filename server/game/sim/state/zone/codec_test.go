package zone_test

import (
	"bytes"
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state/zone"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/shared/binarycodec"
)

func TestAZoneStateComesBackAsItWasWrittenWithTheMapThatItsIDFinds(t *testing.T) {
	st := newState(t)
	st.AddZoneLayer(4, 50)
	st.AddZoneLayer(5, 20)
	st.SendMessage(zone.ChatMessage{FromID: 1, Text: "hello zone", Tick: 3})
	st.SetTick(7)
	tiles := st.Tiles

	var w binarycodec.Writer
	if err := st.Encode(&w); err != nil {
		t.Fatal(err)
	}
	tilesOf := func(id sim.StateID) *tilemap.TileMap {
		if id == 1 {
			return tiles
		}
		return nil
	}
	r := binarycodec.NewReader(w.B)
	got, err := zone.DecodeState(r, 1, tilesOf)
	if err != nil || r.Left() != 0 {
		t.Fatalf("error %v, %d bytes left", err, r.Left())
	}
	if got.Hash() != st.Hash() || got.Tiles != tiles {
		t.Fatal("the state that came back has another hash, or another map")
	}
	if got.WorldID != st.WorldID || !slices.Equal(got.Adjacent, st.Adjacent) || got.WorldID == 0 {
		t.Fatalf("the zone knows world %d and zones %v", got.WorldID, got.Adjacent)
	}
	var again binarycodec.Writer
	if err := got.Encode(&again); err != nil || !bytes.Equal(again.B, w.B) {
		t.Fatalf("the state that came back is written differently: %v", err)
	}
	for n := range w.B { // every proper prefix
		if _, err := zone.DecodeState(binarycodec.NewReader(w.B[:n]), 1, tilesOf); err == nil {
			t.Fatalf("%d of %d bytes were read", n, len(w.B))
		}
	}
	none := func(sim.StateID) *tilemap.TileMap { return nil }
	if _, err := zone.DecodeState(binarycodec.NewReader(w.B), 1, none); err == nil {
		t.Fatal("a zone with no map was read")
	}
}
