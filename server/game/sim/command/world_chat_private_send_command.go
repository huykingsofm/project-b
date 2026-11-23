package command

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// WorldChatPrivateSendCommand asks to send a private message from one player to another.
type WorldChatPrivateSendCommand struct {
	FromID state.PlayerID
	ToID   state.PlayerID
	Text   string
}

func (WorldChatPrivateSendCommand) ID() sim.CommandID { return WorldChatPrivateSendCommandID }

// Clone returns the command itself: it holds only values.
func (c WorldChatPrivateSendCommand) Clone() sim.Command[*worldstate.State] { return c }

func (c WorldChatPrivateSendCommand) Translate() []sim.Action[*worldstate.State] {
	return []sim.Action[*worldstate.State]{
		&action.WorldChatPrivateSendAction{FromID: c.FromID, ToID: c.ToID, Text: c.Text},
	}
}

// Encode writes the fields of the command, not its ID: sim.EncodeCommand writes the ID first.
func (c WorldChatPrivateSendCommand) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(c.FromID))
	w.Uvarint(uint64(c.ToID))
	w.String(c.Text)
	return nil
}

// decodeWorldChatPrivateSendCommand reads what Encode wrote.
func decodeWorldChatPrivateSendCommand(
	r *binary.Reader, version codec.Version,
) (WorldChatPrivateSendCommand, error) {
	c := WorldChatPrivateSendCommand{
		FromID: r.Uint[state.PlayerID](), ToID: r.Uint[state.PlayerID](), Text: r.String(),
	}
	return c, r.Err()
}
