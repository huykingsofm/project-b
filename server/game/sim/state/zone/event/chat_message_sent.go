package event

import (
	"github.com/huykingsofm/project-b/server/game/sim/state"
)

// ChatMessageSent says a message was sent in the zone.
type ChatMessageSent struct {
	FromID state.CharacterID
	Text   string
	Tick   uint64
}

func (ChatMessageSent) ID() ID { return ChatMessageSentID }

// Audience is everyone in the zone.
func (ChatMessageSent) Audience() state.Audience { return state.AudienceEveryone() }
