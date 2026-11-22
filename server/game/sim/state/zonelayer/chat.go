package zonelayer

import (
	"github.com/huykingsofm/project-b/server/game/sim/state"
)

// ChatMessage is something a character said in the zonelayer.
type ChatMessage struct {
	FromID state.CharacterID
	Text   string
	Tick   uint64
}

// Index says by which columns a message is found in a table of messages: who said it.
func (m ChatMessage) Index() map[string]any { return map[string]any{"from": m.FromID} }

// chatKeep is how many messages a zonelayer keeps of what was said in it.
const chatKeep = 100
