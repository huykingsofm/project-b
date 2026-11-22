package zonelayer

import (
	"errors"
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/shared/binarycodec"
)

func playedCharacter() Character {
	c := Character{
		characterID: 7, name: "Binh Nguyên", health: 80, maxHealth: 100, radius: 256, speed: 250,
	}
	c.SetCooldown(9, 1_000_006)
	c.SetCooldown(2, 52_004)
	return c
}

func TestACharacterComesBackAsItWasWrittenWithItsCooldownsInOrder(t *testing.T) {
	c := playedCharacter()
	var w binarycodec.Writer
	c.Encode(&w)
	r := binarycodec.NewReader(w.B)
	got, err := DecodeCharacter(r, 1)
	if err != nil || r.Left() != 0 || !reflect.DeepEqual(got, c) {
		t.Fatalf("got %+v, error %v, %d left; want %+v", got, err, r.Left(), c)
	}
	if keys := got.cooldowns.Keys(); keys[0] != any(CooldownKey(9)) {
		t.Fatalf("keys %v, want 9 first as it was set", keys)
	}
}

func TestACharacterCutOffOrWithAKeyTooLargeIsAnErrorAndNeverAPanic(t *testing.T) {
	var w binarycodec.Writer
	playedCharacter().Encode(&w)
	for cut := range len(w.B) {
		if _, err := DecodeCharacter(binarycodec.NewReader(w.B[:cut]), 1); err == nil {
			t.Fatalf("cut at %d of %d bytes was read", cut, len(w.B))
		}
	}
	var big binarycodec.Writer
	big.Uvarint(1)
	big.String("x")
	for range 4 {
		big.Varint(1)
	}
	big.Uvarint(1)
	big.Uvarint(70000)
	big.Uvarint(5)
	if _, err := DecodeCharacter(binarycodec.NewReader(big.B), 1); !errors.Is(
		err, ErrUnknownCooldownKey,
	) {
		t.Fatalf("a key that is too large: %v", err)
	}
}
