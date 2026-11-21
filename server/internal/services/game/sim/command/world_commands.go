package command

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
)

// ChatPrivateWorldCommand asks to send a private message from one player to another.
type ChatPrivateWorldCommand struct {
	FromID state.PlayerID
	ToID   state.PlayerID
	Text   string
}

func (c ChatPrivateWorldCommand) TranslateWorld() []action.WorldAction {
	return []action.WorldAction{&action.WorldChatPrivateSendAction{FromID: c.FromID, ToID: c.ToID, Text: c.Text}}
}
