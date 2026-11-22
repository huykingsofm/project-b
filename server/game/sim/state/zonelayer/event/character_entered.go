package event

import (
	"github.com/huykingsofm/project-b/server/game/sim/state"
)

// CharacterEntered says a character got a body in the zonelayer.
type CharacterEntered struct {
	EntityID    state.EntityID
	CharacterID state.CharacterID
	Name        string
	Pos         state.Vec
	Radius      int32 // of the body
}

func (CharacterEntered) ID() ID { return CharacterEnteredID }

// Audience is whoever can see any part of the body where it appeared.
func (e CharacterEntered) Audience() state.Audience {
	return state.AudienceInSightOf(state.RectCovering(e.Pos, e.Pos).Grown(e.Radius))
}
