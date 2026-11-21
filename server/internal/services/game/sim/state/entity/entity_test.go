package entity_test

import (
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state/entity"
)

func TestEveryKindHasItsOwnIDAndNoneIsZero(t *testing.T) {
	all := []entity.Entity{entity.CharacterEntity{}, entity.ItemEntity{}, entity.ExitEntity{}}
	seen := map[entity.KindID]entity.Entity{}
	for _, e := range all {
		if e.KindID() == 0 {
			t.Fatalf("%T has the kind 0", e)
		}
		if other, ok := seen[e.KindID()]; ok {
			t.Fatalf("%T and %T have the same kind %d", e, other, e.KindID())
		}
		seen[e.KindID()] = e
	}
}

func TestTheBoundingBoxComesFromThePositionAndTheRadius(t *testing.T) {
	c := entity.CharacterEntity{EntityID: 1, Pos: state.Vec{X: 5000, Y: 4000}, Rad: 2000}
	want := state.Rect{Min: state.Vec{X: 3000, Y: 2000}, Max: state.Vec{X: 7000, Y: 6000}}
	if got := c.BoundingBox(); got != want {
		t.Fatalf("BoundingBox = %+v, want %+v", got, want)
	}
	if got := (entity.ItemEntity{Pos: state.Vec{X: 5, Y: 4}}).BoundingBox(); got.Min != got.Max {
		t.Fatalf("an item of radius 0 fills more than its tile: %+v", got)
	}
}

func TestAnExitIsAnAreaAndItsPositionAndRadiusCoverIt(t *testing.T) {
	e := entity.ExitEntity{Area: state.Rect{Min: state.Vec{X: 9000, Y: 0}, Max: state.Vec{X: 9999, Y: 4999}}}
	if got := e.BoundingBox(); got != e.Area {
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
}

func TestTheHashDependsOnWhatTheEntityIsMadeOf(t *testing.T) {
	hash := func(e entity.Entity) uint64 {
		h := state.NewHasher()
		e.Hash(&h)
		return h.Sum()
	}
	a := entity.ItemEntity{EntityID: 1, Pos: state.Vec{X: 1, Y: 1}, ItemID: 5}
	if hash(a) != hash(a) {
		t.Fatal("the same entity hashes differently")
	}
	for name, b := range map[string]entity.Entity{
		"item kind":   entity.ItemEntity{EntityID: 1, Pos: state.Vec{X: 1, Y: 1}, ItemID: 6},
		"position":    entity.ItemEntity{EntityID: 1, Pos: state.Vec{X: 2, Y: 1}, ItemID: 5},
		"entity kind": entity.CharacterEntity{EntityID: 1, Pos: state.Vec{X: 1, Y: 1}},
	} {
		if hash(a) == hash(b) {
			t.Fatalf("a different %s hashes the same", name)
		}
	}
}

func TestEveryKindCanBeMovedAndTheOriginalDoesNotChange(t *testing.T) {
	to := state.Vec{X: 4500, Y: 4500}
	exit := entity.ExitEntity{Area: state.Rect{Min: state.Vec{X: 9000, Y: 0}, Max: state.Vec{X: 9999, Y: 4999}}}
	for _, e := range []entity.Entity{entity.CharacterEntity{Pos: state.Vec{X: 1, Y: 1}}, entity.ItemEntity{Pos: state.Vec{X: 1, Y: 1}}, exit} {
		moved := e.MovedTo(to)
		if moved.Position() != to {
			t.Fatalf("%T moved to %+v, want %+v", e, moved.Position(), to)
		}
		if e.Position() == to {
			t.Fatalf("moving a %T changed the original", e)
		}
	}
	moved := exit.MovedTo(to).(entity.ExitEntity)
	if moved.Area.Max.Y-moved.Area.Min.Y != 4999 {
		t.Fatalf("moving the exit changed the size of its area: %+v", moved.Area)
	}
}

func TestACharacterCanBeDamagedAndHealedWithinItsHealth(t *testing.T) {
	var d entity.DamageableEntity = entity.CharacterEntity{Hp: 10, MaxHp: 10}
	hurt := d.OnDamage(4)
	if hurt.Health() != 6 || d.Health() != 10 {
		t.Fatalf("after 4 damage: %d (original %d), want 6 and 10", hurt.Health(), d.Health())
	}
	if got := hurt.OnDamage(100).Health(); got != 0 {
		t.Fatalf("health after a big hit = %d, want 0", got)
	}
	if got := hurt.OnHeal(100).Health(); got != 10 {
		t.Fatalf("health after a big heal = %d, want 10", got)
	}
}
