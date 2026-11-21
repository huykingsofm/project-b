package zonelayer

import (
	"fmt"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/shared/orderedmap"
)

// Character is a character that has a body in a zonelayer, and everything the zonelayer knows of it,
// its health included. It travels whole from one zonelayer to another: RemoveCharacterEntity returns it and
// AddCharacterEntity takes it, and the body it had (an entity) is made again when it is added, so the
// character does not know which entity is its body: the state does. Its cooldowns travel with it.
//
// Health, MaxHealth and Radius are those of the character while it is outside a zonelayer. In a zonelayer
// they are on the body (the health on a damageable entity, entity.DamageableEntity), and CharacterByID reads
// them from there.
type Character struct {
	CharacterID state.CharacterID
	Name        string
	Health      int32
	MaxHealth   int32
	// Radius is how far the body of the character reaches from its position, in units. The body is a square
	// of any size: a boss may cover many tiles.
	Radius int32
	// Speed is how many units the character goes in a tick when it moves.
	Speed int32

	// Cooldowns says, for each key, the first tick at which the character is free of what the
	// key stands for. A key is a value of any comparable type. Each action defines a key of its
	// own, as a type of its own (a struct{}), so two actions never share a key by accident, and
	// an action can read the key of another action when it needs to.
	Cooldowns orderedmap.Map[any, uint64]
}

// NewCharacter makes a character that has just been made, with all its health, the speed, in units
// a tick, that it moves at, and the radius of its body, in units. It returns an error, and no character,
// when a value cannot be (a negative radius): what is outside the gateway is not trusted, and the caller
// logs the error and goes no further.
func NewCharacter(characterID state.CharacterID, name string, maxHealth int32, speed int32, radius int32) (Character, error) {
	if radius < 0 {
		return Character{}, fmt.Errorf("zonelayer: the radius of a body is not negative, got %d", radius)
	}
	return Character{CharacterID: characterID, Name: name, Health: maxHealth, MaxHealth: maxHealth, Speed: speed, Radius: radius}, nil
}
