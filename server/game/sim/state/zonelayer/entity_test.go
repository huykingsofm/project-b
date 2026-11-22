package zonelayer

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
)

func TestEveryKindHasItsOwnIDAndNoneIsZero(t *testing.T) {
	all := []Entity{CharacterEntity{}, ItemEntity{}}
	seen := map[KindID]Entity{}
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
	c := CharacterEntity{entityID: 1, pos: state.Vec{X: 5000, Y: 4000}, rad: 2000}
	want := state.Rect{Min: state.Vec{X: 3000, Y: 2000}, Max: state.Vec{X: 7000, Y: 6000}}
	if got := c.BoundingBox(); got != want {
		t.Fatalf("BoundingBox = %+v, want %+v", got, want)
	}
	if got := (ItemEntity{pos: state.Vec{X: 5, Y: 4}}).BoundingBox(); got.Min != got.Max {
		t.Fatalf("an item of radius 0 fills more than its tile: %+v", got)
	}
}

func TestTheHashDependsOnWhatTheEntityIsMadeOf(t *testing.T) {
	hash := func(e Entity) uint64 {
		h := sim.NewHasher()
		e.Hash(&h)
		return h.Sum()
	}
	a := ItemEntity{entityID: 1, pos: state.Vec{X: 1, Y: 1}, itemID: 5}
	h1 := hash(a)
	h2 := hash(a)
	if h1 != h2 {
		t.Fatal("the same entity hashes differently")
	}
	for name, b := range map[string]Entity{
		"item kind":   ItemEntity{entityID: 1, pos: state.Vec{X: 1, Y: 1}, itemID: 6},
		"position":    ItemEntity{entityID: 1, pos: state.Vec{X: 2, Y: 1}, itemID: 5},
		"entity kind": CharacterEntity{entityID: 1, pos: state.Vec{X: 1, Y: 1}},
	} {
		if hash(a) == hash(b) {
			t.Fatalf("a different %s hashes the same", name)
		}
	}
}

func TestEveryKindCanBeMovedAndTheOriginalDoesNotChange(t *testing.T) {
	to := state.Vec{X: 4500, Y: 4500}
	for _, e := range []Entity{
		CharacterEntity{pos: state.Vec{X: 1, Y: 1}},
		ItemEntity{pos: state.Vec{X: 1, Y: 1}},
	} {
		moved := e.MovedTo(to)
		if moved.Position() != to {
			t.Fatalf("%T moved to %+v, want %+v", e, moved.Position(), to)
		}
		if e.Position() == to {
			t.Fatalf("moving a %T changed the original", e)
		}
	}
}

func TestACharacterCanBeDamagedAndHealedWithinItsHealth(t *testing.T) {
	var d DamageableEntity = CharacterEntity{hp: 10, maxHp: 10}
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
