package event

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// PrivateChatMessageSent says a player sent a private message to another player.
type PrivateChatMessageSent struct {
	FromID state.PlayerID
	ToID   state.PlayerID
	Text   string
	Tick   uint64
}

func (PrivateChatMessageSent) ID() ID { return PrivateChatMessageSentID }
