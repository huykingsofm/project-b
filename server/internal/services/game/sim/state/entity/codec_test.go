package entity

import (
	"errors"
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

func everyEntity() []Entity {
	at := state.Vec{X: -300, Y: 9000}
	return []Entity{
		CharacterEntity{EntityID: 3, Pos: at, Rad: 256, Hp: 80, MaxHp: 100},
		ItemEntity{EntityID: 4, Pos: at, Rad: 8, ItemID: 7, Count: 5, Expiry: 90},
		ExitEntity{
			EntityID: 5,
			Area:     state.Rect{Min: state.Vec{X: 9, Y: 0}, Max: state.Vec{X: 9, Y: 4}},
			ToZoneID: 2, Arrival: state.Vec{X: 1, Y: 2},
		},
	}
}

func TestEveryKindOfEntityHasADecoderAndComesBackAsItWas(t *testing.T) {
	for kind := CharacterKindID; kind <= ExitKindID; kind++ { // the last kind of the register
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
	if _, err := Decode(binarycodec.NewReader(w.B), 1); !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("the number 99: %v, want ErrUnknownKind", err)
	}
}
