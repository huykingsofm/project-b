package zonelayer

import (
	"github.com/huykingsofm/project-b/server/game/sim/state"
)

// chatKeep is how many messages a zonelayer keeps of what was said in it.
const chatKeep = 100

// ChatMessage is something a character said in the zonelayer.
type ChatMessage struct {
	fromID state.CharacterID
	text   string
	tick   uint64
}

// NewChatMessage makes a message that a character said in a zonelayer at a tick.
func NewChatMessage(fromID state.CharacterID, text string, tick uint64) ChatMessage {
	return ChatMessage{
		fromID: fromID, text: text, tick: tick,
	}
}
