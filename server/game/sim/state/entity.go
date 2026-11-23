package state

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// KindID names a kind of entity, by a number. A number is never changed or reused, because
// checkpoints and the replay log write it; 0 is no kind. Every kind of the game is in this one
// list, so a kind means one thing whichever state holds it, and each state holds the kinds that it
// chooses (the entities of a kind are in the package entity below the state).
type KindID uint16

// The kinds.
const (
	CharacterKindID KindID = iota + 1
	ItemKindID
	ExitKindID
)

// Entity is what stands in a state: a body, an item on the ground, an exit. Every entity is a
// value type (R29): a change builds a new value and replaces the old one in its container.
type Entity interface {
	ID() EntityID
	KindID() KindID
	Position() Vec
	Radius() int32
	BoundingBox() Rect
	// MovedTo returns the entity at another position; the receiver does not change.
	MovedTo(pos Vec) Entity
	// Hash adds the entity to the hash of its state.
	Hash(h *sim.Hasher)
	// Encode writes the fields of the entity, not its kind: the codec of its package writes the
	// kind first.
	Encode(w *binary.Writer)
}

// DamageableEntity is an Entity with health. A state damages and heals through it and never asks
// the kind.
type DamageableEntity interface {
	Entity
	Health() int32
	MaxHealth() int32
	// OnDamage returns the entity with health lowered, never below 0.
	OnDamage(amount int32) DamageableEntity
	// OnHeal returns the entity with health raised, never above MaxHealth.
	OnHeal(amount int32) DamageableEntity
}

// BoundingBoxAround is the smallest rectangle that holds a circle of the radius at the position.
func BoundingBoxAround(pos Vec, radius int32) Rect {
	return Rect{
		Min: Vec{X: pos.X - radius, Y: pos.Y - radius},
		Max: Vec{X: pos.X + radius, Y: pos.Y + radius},
	}
}

// HashEntity adds what every entity has to a hash: its ID, its kind and its bounding box. The
// entity adds the fields of its own kind after it.
func HashEntity(h *sim.Hasher, e Entity) {
	h.U64(uint64(e.ID()))
	h.U64(uint64(e.KindID()))
	box := e.BoundingBox()
	for _, n := range [...]int32{box.Min.X, box.Min.Y, box.Max.X, box.Max.Y} {
		h.U64(uint64(uint32(n)))
	}
}
