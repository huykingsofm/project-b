package zonelayer

import (
	"errors"
	"fmt"

	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/shared/binarycodec"
	"github.com/huykingsofm/project-b/shared/orderedmap"
)

// CooldownKey names what a cooldown is for. The register of the keys, with their numbers, is in
// package action.
type CooldownKey uint16

// Character is a character that has a body in a zonelayer, and everything the zonelayer knows of
// it, its health included. It travels whole from one zonelayer to another: RemoveCharacterEntity
// returns it and AddCharacterEntity takes it, and the body it had (an entity) is made again when it
// is added, so the character does not know which entity is its body: the state does. Its cooldowns
// travel with it.
//
// Health, MaxHealth and Radius are those of the character while it is outside a zonelayer. In a
// zonelayer they are on the body (the health on a damageable entity, entity.DamageableEntity), and
// CharacterByID reads them from there.
type Character struct {
	characterID state.CharacterID
	name        string
	health      int32
	maxHealth   int32
	// radius is how far the body of the character reaches from its position, in units. The body is
	// a square of any size: a boss may cover many tiles.
	radius int32
	// speed is how many units the character goes in a tick when it moves.
	speed int32

	// cooldowns says, for each key, the first tick at which the character is free of what the
	// key stands for. A key is a number of the register of the cooldown keys (see package action).
	cooldowns orderedmap.Map[CooldownKey, uint64]
}

// Clone returns a copy of the character that shares nothing with it: its cooldowns are copied,
// because they hold a map and slices that assignment does not copy.
func (c Character) Clone() Character {
	c.cooldowns = c.cooldowns.Clone()
	return c
}

// NewCharacter makes a character that has just been made, with all its health, the speed, in units
// a tick, that it moves at, and the radius of its body, in units. It returns an error, and no
// character, when a value cannot be (a negative radius): what is outside the gateway is not
// trusted, and the caller logs the error and goes no further.
func NewCharacter(
	characterID state.CharacterID, name string, maxHealth int32, speed int32, radius int32,
) (Character, error) {
	if radius < 0 {
		return Character{}, fmt.Errorf(
			"zonelayer: the radius of a body is not negative, got %d", radius,
		)
	}
	return Character{
		characterID: characterID,
		name:        name,
		health:      maxHealth,
		maxHealth:   maxHealth,
		speed:       speed,
		radius:      radius,
	}, nil
}

// ErrUnknownCooldownKey is returned when bytes name a cooldown key that is too large to be the
// number of a key.
var ErrUnknownCooldownKey = errors.New("zonelayer: the number is not one of a cooldown key")

// Encode writes a character: its fields and its cooldowns, each as the number of its key and the
// tick it ends at, in the order they were set.
func (c Character) Encode(w *binarycodec.Writer) {
	w.Uvarint(uint64(c.characterID))
	w.String(c.name)
	w.Varint(int64(c.health))
	w.Varint(int64(c.maxHealth))
	w.Varint(int64(c.radius))
	w.Varint(int64(c.speed))
	w.Uvarint(uint64(c.cooldowns.Len()))
	for key, until := range c.cooldowns.All() {
		w.Uvarint(uint64(key))
		w.Uvarint(until)
	}
}

// DecodeCharacter reads what Character.Encode wrote, of the data version given. It returns an error
// of the bytes (binarycodec.ErrShort), or one that wraps ErrUnknownCooldownKey.
func DecodeCharacter(r *binarycodec.Reader, version binarycodec.Version) (Character, error) {
	c := Character{
		characterID: state.CharacterID(r.Uvarint()),
		name:        r.String(),
		health:      int32(r.Varint()),
		maxHealth:   int32(r.Varint()),
		radius:      int32(r.Varint()),
		speed:       int32(r.Varint()),
	}
	n := r.Uvarint()
	for i := uint64(0); i < n && r.Err() == nil; i++ {
		code, until := r.Uvarint(), r.Uvarint()
		if r.Err() != nil {
			break
		}
		if code > 0xffff {
			return Character{}, fmt.Errorf("%w: %d", ErrUnknownCooldownKey, code)
		}
		c.cooldowns.Set(CooldownKey(code), until)
	}
	return c, r.Err()
}

// CharacterID is the ID of the character.
func (c Character) CharacterID() state.CharacterID { return c.characterID }

// Name is the name of the character.
func (c Character) Name() string { return c.name }

// Health is the health of the character; once it is in a zonelayer it is on the body.
func (c Character) Health() int32 { return c.health }

// MaxHealth is the most health of the character.
func (c Character) MaxHealth() int32 { return c.maxHealth }

// Radius is the radius of the body of the character, in units.
func (c Character) Radius() int32 { return c.radius }

// Speed is how far the character moves in a tick, in units.
func (c Character) Speed() int32 { return c.speed }

// CooldownUntil is the tick at which the character is free of what the key stands for, and false
// when the character has no cooldown for it.
func (c Character) CooldownUntil(key CooldownKey) (uint64, bool) {
	return c.cooldowns.Get(key)
}

// CooldownKeys returns the keys of the cooldowns of the character, in the order they were set.
func (c Character) CooldownKeys() []CooldownKey { return c.cooldowns.Keys() }

// SetCooldown sets the tick at which the character is free of what the key stands for.
func (c *Character) SetCooldown(key CooldownKey, until uint64) {
	c.cooldowns.Set(key, until)
}

// SetHealth sets the health of the character, before it enters a zonelayer.
func (c *Character) SetHealth(health int32) { c.health = health }

// SetSpeed sets how far the character moves in a tick, in units.
func (c *Character) SetSpeed(speed int32) { c.speed = speed }
