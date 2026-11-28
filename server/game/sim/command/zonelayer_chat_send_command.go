package command

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ZoneLayerChatSendCommand asks to say something to the zonelayer the character is in.
type ZoneLayerChatSendCommand struct {
	FromID state.CharacterID
	Text   string
}

// decodeZoneLayerChatSendCommand reads what Encode wrote.
func decodeZoneLayerChatSendCommand(
	r *binary.Reader, version codec.Version,
) (ZoneLayerChatSendCommand, error) {
	c := ZoneLayerChatSendCommand{FromID: r.Uint[state.CharacterID](), Text: r.String()}
	return c, r.Err()
}

func (ZoneLayerChatSendCommand) ID() sim.CommandID { return ZoneLayerChatSendCommandID }

// Clone returns the command itself: it holds only values.
func (c ZoneLayerChatSendCommand) Clone() sim.Command[*zonelayerstate.State] { return c }

func (c ZoneLayerChatSendCommand) Translate() []sim.Action[*zonelayerstate.State] {
	return []sim.Action[*zonelayerstate.State]{
		action.ZoneLayerChatSendAction{FromID: c.FromID, Text: c.Text},
	}
}

// Encode writes the fields of the command, not its ID: sim.EncodeCommand writes the ID first.
func (c ZoneLayerChatSendCommand) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(c.FromID))
	w.String(c.Text)
	return nil
}
