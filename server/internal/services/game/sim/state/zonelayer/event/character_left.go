package event

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// CharacterLeft says the body of a character is gone from the zonelayer.
type CharacterLeft struct {
	EntityID state.EntityID
	At       state.Vec // where the body was
	Radius   int32     // of the body
}

func (CharacterLeft) ID() ID { return CharacterLeftID }

// Audience is whoever could see any part of the body where it was.
func (e CharacterLeft) Audience() state.Audience {
	return state.AudienceInSightOf(state.RectCovering(e.At, e.At).Grown(e.Radius))
}
