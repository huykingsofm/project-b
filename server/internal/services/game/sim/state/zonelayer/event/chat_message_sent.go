package event

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// ChatMessageSent says a message was sent in the zonelayer.
type ChatMessageSent struct {
	FromID state.CharacterID
	Text   string
	Tick   uint64
}

func (ChatMessageSent) ID() ID { return ChatMessageSentID }
