package entity_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/zone/entity"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// theZone is the zone that the exits of these tests are in.
var theZone = sim.NewStateID(state.ZoneType, 1)

func anExit() entity.ExitEntity {
	return entity.NewExitEntity(
		state.EntityID(5),
		state.Rect{Min: state.Vec{X: 9000, Y: 0}, Max: state.Vec{X: 9999, Y: 4999}}, 2,
		state.Vec{X: 1, Y: 2},
	)
}

func TestAnExitIsAnAreaAndItsPositionAndRadiusCoverIt(t *testing.T) {
	e := anExit()
	if got := e.BoundingBox(); got != e.Area() {
		t.Fatalf("BoundingBox = %+v, want the area", got)
	}
	if got := e.Position(); got != (state.Vec{X: 9499, Y: 2499}) {
		t.Fatalf("Position = %+v, want the middle of the area", got)
	}
	if got := e.Radius(); got != 2500 {
		t.Fatalf("Radius = %d, want 2500", got)
	}
	if !e.Contains(state.Vec{X: 9500, Y: 3500}) || e.Contains(state.Vec{X: 8500, Y: 3500}) {
		t.Fatal("Contains does not follow the area")
	}
	arrival := state.Vec{X: 1, Y: 2}
	if e.KindID() != state.ExitKindID || e.ToZoneID() != 2 || e.Arrival() != arrival {
		t.Fatal("the accessors do not say what the exit was made of")
	}
}

func TestAnExitMovedKeepsItsSizeAndItsArrivalAndTheOriginalDoesNotChange(t *testing.T) {
	e := anExit()
	to := state.Vec{X: 4500, Y: 4500}
	moved := e.MovedTo(to).(entity.ExitEntity)
	if moved.Position() != to || e.Position() == to {
		t.Fatalf("moved to %+v (original at %+v), want %+v", moved.Position(), e.Position(), to)
	}
	if moved.Area().Max.Y-moved.Area().Min.Y != 4999 || moved.Arrival() != e.Arrival() {
		t.Fatalf("moving the exit changed its size or its arrival: %+v", moved)
	}
}

func TestTheHashOfAnExitDependsOnWhereItLeadsAndWhereItArrives(t *testing.T) {
	hash := func(e entity.ExitEntity) uint64 {
		h := sim.NewHasher()
		e.Hash(&h)
		return h.Sum()
	}
	a := anExit()
	other := entity.NewExitEntity(a.ID(), a.Area(), 3, a.Arrival())
	late := entity.NewExitEntity(a.ID(), a.Area(), a.ToZoneID(), state.Vec{X: 9, Y: 9})
	if hash(a) != hash(anExit()) || hash(a) == hash(other) || hash(a) == hash(late) {
		t.Fatal("the hash does not follow the zone it leads to and the place it arrives at")
	}
}

func TestAnExitComesBackAsItWasWrittenAndCutBytesAndOtherKindsAreAnError(t *testing.T) {
	var w binary.Writer
	if err := entity.Encode(&w, anExit()); err != nil {
		t.Fatal(err)
	}
	r := binary.NewReader(w.B)
	got, err := entity.Decode(r, 1)
	if err != nil || r.Left() != 0 || !reflect.DeepEqual(got, anExit()) {
		t.Fatalf("came back as %+v, error %v, %d bytes left", got, err, r.Left())
	}
	for n := range w.B {
		if _, err := entity.Decode(binary.NewReader(w.B[:n]), 1); err == nil {
			t.Fatalf("%d of %d bytes were read", n, len(w.B))
		}
	}
	if err := entity.Encode(&binary.Writer{}, nil); !errors.Is(err, entity.ErrNoCodec) {
		t.Fatalf("nil: %v", err)
	}
	var item binary.Writer
	item.Uvarint(uint64(state.ItemKindID)) // a kind that a zone does not hold
	if _, err := entity.Decode(binary.NewReader(item.B), 1); !errors.Is(err,
		entity.ErrUnknownKind) {
		t.Fatalf("an item in a zone: %v", err)
	}
}
