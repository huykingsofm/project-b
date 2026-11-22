package command

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// WorldChatPrivateSendCommand asks to send a private message from one player to another.
type WorldChatPrivateSendCommand struct {
	FromID state.PlayerID
	ToID   state.PlayerID
	Text   string
}

func (WorldChatPrivateSendCommand) ID() ID { return WorldChatPrivateSendCommandID }

// Clone returns the command itself: it holds only values.
func (c WorldChatPrivateSendCommand) Clone() WorldCommand { return c }

func (c WorldChatPrivateSendCommand) TranslateWorld() []action.WorldAction {
	return []action.WorldAction{
		&action.WorldChatPrivateSendAction{FromID: c.FromID, ToID: c.ToID, Text: c.Text},
	}
}

// Encode writes the fields of the command, not its ID: package command writes the ID first.
func (c WorldChatPrivateSendCommand) Encode(w *binarycodec.Writer) error {
	w.Uvarint(uint64(c.FromID))
	w.Uvarint(uint64(c.ToID))
	w.String(c.Text)
	return nil
}

// decodeWorldChatPrivateSendCommand reads what Encode wrote.
func decodeWorldChatPrivateSendCommand(
	r *binarycodec.Reader, version binarycodec.Version,
) (Command, error) {
	c := WorldChatPrivateSendCommand{
		FromID: state.PlayerID(r.Uvarint()), ToID: state.PlayerID(r.Uvarint()), Text: r.String(),
	}
	return c, r.Err()
}
