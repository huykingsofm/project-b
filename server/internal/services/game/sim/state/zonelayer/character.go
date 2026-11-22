package zonelayer

import (
	"errors"
	"fmt"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
	"github.com/huykingsofm/project-b/server/internal/shared/orderedmap"
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
	CharacterID state.CharacterID
	Name        string
	Health      int32
	MaxHealth   int32
	// Radius is how far the body of the character reaches from its position, in units. The body is
	// a square of any size: a boss may cover many tiles.
	Radius int32
	// Speed is how many units the character goes in a tick when it moves.
	Speed int32

	// Cooldowns says, for each key, the first tick at which the character is free of what the
	// key stands for. A key is a number of the register of the cooldown keys (see package action).
	Cooldowns orderedmap.Map[CooldownKey, uint64]
}

// Clone returns a copy of the character that shares nothing with it: its cooldowns are copied,
// because they hold a map and slices that assignment does not copy.
func (c Character) Clone() Character {
	c.Cooldowns = c.Cooldowns.Clone()
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
		CharacterID: characterID,
		Name:        name,
		Health:      maxHealth,
		MaxHealth:   maxHealth,
		Speed:       speed,
		Radius:      radius,
	}, nil
}

// ErrUnknownCooldownKey is returned when bytes name a cooldown key that is too large to be the
// number of a key.
var ErrUnknownCooldownKey = errors.New("zonelayer: the number is not one of a cooldown key")

// Encode writes a character: its fields and its cooldowns, each as the number of its key and the
// tick it ends at, in the order they were set.
func (c Character) Encode(w *binarycodec.Writer) {
	w.Uvarint(uint64(c.CharacterID))
	w.String(c.Name)
	w.Varint(int64(c.Health))
	w.Varint(int64(c.MaxHealth))
	w.Varint(int64(c.Radius))
	w.Varint(int64(c.Speed))
	w.Uvarint(uint64(c.Cooldowns.Len()))
	for key, until := range c.Cooldowns.All() {
		w.Uvarint(uint64(key))
		w.Uvarint(until)
	}
}

// DecodeCharacter reads what Character.Encode wrote, of the data version given. It returns an error
// of the bytes (binarycodec.ErrShort), or one that wraps ErrUnknownCooldownKey.
func DecodeCharacter(r *binarycodec.Reader, version binarycodec.Version) (Character, error) {
	c := Character{
		CharacterID: state.CharacterID(r.Uvarint()),
		Name:        r.String(),
		Health:      int32(r.Varint()),
		MaxHealth:   int32(r.Varint()),
		Radius:      int32(r.Varint()),
		Speed:       int32(r.Varint()),
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
		c.Cooldowns.Set(CooldownKey(code), until)
	}
	return c, r.Err()
}
