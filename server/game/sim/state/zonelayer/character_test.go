package zonelayer_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

func playedCharacter() zonelayer.Character {
	c, err := zonelayer.NewCharacter(7, "Binh Nguyên", 100, 250, 256)
	if err != nil {
		panic(err)
	}
	c.SetHealth(80)
	c.SetCooldown(9, 1_000_006)
	c.SetCooldown(2, 52_004)
	return c
}

func TestACharacterComesBackAsItWasWrittenWithItsCooldownsInOrder(t *testing.T) {
	c := playedCharacter()
	var w binary.Writer
	c.Encode(&w)
	r := binary.NewReader(w.B)
	got, err := zonelayer.DecodeCharacter(r, 1)
	if err != nil || r.Left() != 0 || !reflect.DeepEqual(got, c) {
		t.Fatalf("got %+v, error %v, %d left; want %+v", got, err, r.Left(), c)
	}
	if keys := got.CooldownKeys(); keys[0] != zonelayer.CooldownKey(9) {
		t.Fatalf("keys %v, want 9 first as it was set", keys)
	}
}

func TestACharacterCutOffOrWithAKeyTooLargeIsAnErrorAndNeverAPanic(t *testing.T) {
	var w binary.Writer
	playedCharacter().Encode(&w)
	for cut := range len(w.B) {
		if _, err := zonelayer.DecodeCharacter(binary.NewReader(w.B[:cut]), 1); err == nil {
			t.Fatalf("cut at %d of %d bytes was read", cut, len(w.B))
		}
	}
	var big binary.Writer
	big.Uvarint(1)
	big.String("x")
	for range 4 {
		big.Varint(1)
	}
	big.Uvarint(1)
	big.Uvarint(70000)
	big.Uvarint(5)
	if _, err := zonelayer.DecodeCharacter(binary.NewReader(big.B), 1); !errors.Is(
		err, zonelayer.ErrUnknownCooldownKey,
	) {
		t.Fatalf("a key that is too large: %v", err)
	}
}
