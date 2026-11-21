package event

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// CharacterEntered says a character got a body in the zonelayer.
type CharacterEntered struct {
	EntityID state.EntityID
	CharID   state.CharacterID
	Name     string
	Pos      state.Vec
}

func (CharacterEntered) ID() ID { return CharacterEnteredID }
