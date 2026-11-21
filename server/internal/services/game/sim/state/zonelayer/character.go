package zonelayer

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/shared/orderedmap"
)

// Character is a character that has a body in a zonelayer, and everything the zonelayer knows of it,
// its health included. It travels whole from one zonelayer to another: DetachCharacter returns it and
// AttachCharacter takes it, and the body it had (Entity) is made again when it is attached, so the
// character does not know which entity is its body: the state does. Its
// cooldowns travel with it.
type Character struct {
	CharID    state.CharacterID
	Name      string
	Health    int32
	MaxHealth int32

	// Cooldowns says, for each key, the first tick at which the character is free of what the
	// key stands for. A key is a value of any comparable type. Each action defines a key of its
	// own, as a type of its own (a struct{}), so two actions never share a key by accident, and
	// an action can read the key of another action when it needs to.
	Cooldowns orderedmap.Map[any, uint64]
}

// NewCharacter makes a character that has just been made, with all its health.
func NewCharacter(charID state.CharacterID, name string, maxHealth int32) Character {
	return Character{CharID: charID, Name: name, Health: maxHealth, MaxHealth: maxHealth}
}
