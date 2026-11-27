package entity_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/zonelayer/entity"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// theLayer is the zonelayer that the entities of these tests are in.
var theLayer = sim.NewStateID(state.ZoneLayerType, 1)

// aLayerEntityID is the ID of the entity that the zonelayer gave the number local.
func aLayerEntityID(local uint64) state.EntityID { return state.EntityID(local) }

func everyEntity() []state.Entity {
	at := state.Vec{X: -300, Y: 9000}
	return []state.Entity{
		entity.NewCharacterEntity(aLayerEntityID(3), at, 256, 80, 100),
		entity.NewItemEntity(aLayerEntityID(4), at, 7, 5, 90),
	}
}

func TestEveryKindOfEntityHasADecoderAndComesBackAsItWas(t *testing.T) {
	for _, e := range everyEntity() {
		var w binary.Writer
		if err := entity.Encode(&w, e); err != nil {
			t.Fatalf("%T: %v", e, err)
		}
		r := binary.NewReader(w.B)
		got, err := entity.Decode(r, 1)
		if err != nil || r.Left() != 0 || !reflect.DeepEqual(got, e) {
			t.Fatalf("%T came back as %+v, error %v, %d bytes left", e, got, err, r.Left())
		}
		for n := range w.B { // every proper prefix
			if _, err := entity.Decode(binary.NewReader(w.B[:n]), 1); err == nil {
				t.Fatalf("%T: %d of %d bytes were read", e, n, len(w.B))
			}
		}
	}
}

func TestANilEntityAndANumberThatIsNoKindAreRefused(t *testing.T) {
	var w binary.Writer
	if err := entity.Encode(&w, nil); !errors.Is(err, entity.ErrNoCodec) || len(w.B) != 0 {
		t.Fatalf("a nil entity: %v, %d bytes written", err, len(w.B))
	}
	w.Uvarint(99)
	_, err := entity.Decode(binary.NewReader(w.B), 1)
	if !errors.Is(err, entity.ErrUnknownKind) {
		t.Fatalf("the number 99: %v, want ErrUnknownKind", err)
	}
}
