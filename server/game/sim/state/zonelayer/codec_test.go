package zonelayer_test

import (
	"bytes"
	"testing"

	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/binarycodec"
)

// aStateWithEverything has characters with cooldowns, items, a body that moved and what was said.
func aStateWithEverything(t testing.TB) *zonelayer.State {
	st := zonelayer.NewState(3, 2, 1)
	for _, id := range []state.CharacterID{1, 2} {
		c, err := zonelayer.NewCharacter(id, "Binh", 100, 250, 256)
		if err != nil {
			t.Fatal(err)
		}
		c.Cooldowns.Set(cooldownKey, 40+uint64(id))
		st.AddCharacterEntity(c, state.Vec{X: 2 * tile, Y: 2 * tile})
	}
	st.AddItemEntity(7, 5, state.Vec{X: 3 * tile, Y: 3 * tile}, 90)
	body, _ := st.CharacterEntityByCharacterID(1)
	st.MoveEntity(map10x5(t), body.EntityID, state.East, 300)
	st.DamageEntity(body.EntityID, 30)
	st.SendMessage(zonelayer.ChatMessage{FromID: 1, Text: "xin chào", Tick: 4})
	st.SetTick(12)
	return st
}

func TestAZoneLayerStateComesBackAsItWasWrittenAndCutBytesAreAnError(t *testing.T) {
	st := aStateWithEverything(t)
	var w binarycodec.Writer
	if err := st.Encode(&w); err != nil {
		t.Fatal(err)
	}
	r := binarycodec.NewReader(w.B)
	got, err := zonelayer.DecodeState(r, 1)
	if err != nil || r.Left() != 0 {
		t.Fatalf("error %v, %d bytes left", err, r.Left())
	}
	if got.Hash() != st.Hash() {
		t.Fatal("the state that came back has another hash")
	}
	if got.WorldID != 1 || got.ZoneID != 2 || got.ID != 3 {
		t.Fatalf("the zonelayer knows zone %d and world %d", got.ZoneID, got.WorldID)
	}
	var again binarycodec.Writer
	if err := got.Encode(&again); err != nil || !bytes.Equal(again.B, w.B) {
		t.Fatalf("the state that came back is written differently: %v", err)
	}
	if got.CharacterCount() != 2 {
		t.Fatalf("%d characters came back, want 2", got.CharacterCount())
	}
	// the state goes on as the original does: a new entity takes the ID that it would have taken
	if a, b := st.AddItemEntity(8, 1, state.Vec{X: tile, Y: tile}, 0),
		got.AddItemEntity(8, 1, state.Vec{X: tile, Y: tile}, 0); a != b {
		t.Fatalf("the next entity gets the ID %d, and %d in the original", b, a)
	}
	for n := range w.B { // every proper prefix
		if _, err := zonelayer.DecodeState(binarycodec.NewReader(w.B[:n]), 1); err == nil {
			t.Fatalf("%d of %d bytes were read", n, len(w.B))
		}
	}
}
