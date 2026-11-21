// Package entity holds the entities that stand in a zonelayer: what a character is, an item on the
// ground and an exit have in common, and each kind in a file of its own. An entity is a value (R29):
// a change builds a new value and replaces the old one in its container.
package entity

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// KindID names a kind of entity, so that a client or an encoder can tell the kinds apart by number.
// The numbers count up from 1 and 0 is no kind. A new kind is added at the end of the list, and a
// number is never changed or reused.
type KindID uint16

// The kinds of entity.
const (
	CharacterKindID KindID = iota + 1
	ItemKindID
	ExitKindID
)

// Entity is something that stands in a zonelayer. It is implemented only by value types.
type Entity interface {
	// ID names the entity in its zonelayer.
	ID() state.EntityID
	// KindID says which kind of entity it is.
	KindID() KindID
	// Position is where the entity stands, in units.
	Position() state.Vec
	// Radius is how far the entity reaches from its position, in units; 0 is a point. Collision uses it.
	Radius() int32
	// BoundingBox is the smallest rectangle that holds the entity, which a grid files it by.
	BoundingBox() state.Rect
	// MovedTo returns the entity at another position; the entity itself does not change.
	MovedTo(pos state.Vec) Entity
	// Hash adds what the entity is made of to the hash of the state.
	Hash(h *state.Hasher)
}

// DamageableEntity is an entity that has health and can lose and regain it. An entity says that it is
// damageable by implementing this interface; the state asks no more than that, so a new kind of entity
// with health is made damageable the same way.
type DamageableEntity interface {
	Entity
	// Health is the health the entity has now.
	Health() int32
	// MaxHealth is the most health the entity can have.
	MaxHealth() int32
	// OnDamage returns the entity after it took damage, with its health never below zero.
	OnDamage(amount int32) DamageableEntity
	// OnHeal returns the entity after it got health back, never above its most.
	OnHeal(amount int32) DamageableEntity
}

// boundingBoxAround is the rectangle that holds a position and the tiles around it within a radius.
func boundingBoxAround(pos state.Vec, radius int32) state.Rect {
	return state.Rect{
		Min: state.Vec{X: pos.X - radius, Y: pos.Y - radius},
		Max: state.Vec{X: pos.X + radius, Y: pos.Y + radius},
	}
}

func hashCommon(h *state.Hasher, e Entity) {
	h.U64(uint64(e.ID()))
	h.U64(uint64(e.KindID()))
	box := e.BoundingBox()
	for _, n := range [...]int32{box.Min.X, box.Min.Y, box.Max.X, box.Max.Y} {
		h.U64(uint64(uint32(n)))
	}
}
