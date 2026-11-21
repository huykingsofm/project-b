package event

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// CharacterLeft says the body of a character is gone from the zonelayer.
type CharacterLeft struct{ EntityID state.EntityID }

func (CharacterLeft) ID() ID { return CharacterLeftID }
