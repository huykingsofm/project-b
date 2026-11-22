package entity

import (
	"errors"
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/shared/binarycodec"
)

func everyEntity() []state.Entity {
	at := state.Vec{X: -300, Y: 9000}
	return []state.Entity{
		CharacterEntity{entityID: 3, pos: at, rad: 256, hp: 80, maxHp: 100},
		ItemEntity{entityID: 4, pos: at, rad: 8, itemID: 7, count: 5, expiry: 90},
	}
}

func TestEveryKindOfEntityHasADecoderAndComesBackAsItWas(t *testing.T) {
	// the last kind that a zonelayer holds
	for kind := state.CharacterKindID; kind <= state.ItemKindID; kind++ {
		if _, ok := decoders[kind]; !ok {
			t.Errorf("the kind %d has no decoder", kind)
		}
	}
	for _, e := range everyEntity() {
		var w binarycodec.Writer
		if err := Encode(&w, e); err != nil {
			t.Fatalf("%T: %v", e, err)
		}
		r := binarycodec.NewReader(w.B)
		got, err := Decode(r, 1)
		if err != nil || r.Left() != 0 || !reflect.DeepEqual(got, e) {
			t.Fatalf("%T came back as %+v, error %v, %d bytes left", e, got, err, r.Left())
		}
		for n := range w.B { // every proper prefix
			if _, err := Decode(binarycodec.NewReader(w.B[:n]), 1); err == nil {
				t.Fatalf("%T: %d of %d bytes were read", e, n, len(w.B))
			}
		}
	}
}

func TestANilEntityAndANumberThatIsNoKindAreRefused(t *testing.T) {
	var w binarycodec.Writer
	if err := Encode(&w, nil); !errors.Is(err, ErrNoCodec) || len(w.B) != 0 {
		t.Fatalf("a nil entity: %v, %d bytes written", err, len(w.B))
	}
	w.Uvarint(99)
	_, err := Decode(binarycodec.NewReader(w.B), 1)
	if !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("the number 99: %v, want ErrUnknownKind", err)
	}
}
