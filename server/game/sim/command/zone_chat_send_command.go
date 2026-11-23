package command

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	"github.com/huykingsofm/project-b/shared/binarycodec"
)

// ZoneChatSendCommand asks to say something to the zone.
type ZoneChatSendCommand struct {
	FromID state.CharacterID
	Text   string
}

func (ZoneChatSendCommand) ID() sim.CommandID { return ZoneChatSendCommandID }

// Clone returns the command itself: it holds only values.
func (c ZoneChatSendCommand) Clone() sim.Command[*zonestate.State] { return c }

func (c ZoneChatSendCommand) Translate() []sim.Action[*zonestate.State] {
	return []sim.Action[*zonestate.State]{
		&action.ZoneChatSendAction{FromID: c.FromID, Text: c.Text},
	}
}

// Encode writes the fields of the command, not its ID: sim.EncodeCommand writes the ID first.
func (c ZoneChatSendCommand) Encode(w *binarycodec.Writer) error {
	w.Uvarint(uint64(c.FromID))
	w.String(c.Text)
	return nil
}

// decodeZoneChatSendCommand reads what Encode wrote.
func decodeZoneChatSendCommand(
	r *binarycodec.Reader, version binarycodec.Version,
) (sim.Command[*zonestate.State], error) {
	c := ZoneChatSendCommand{FromID: r.Uint[state.CharacterID](), Text: r.String()}
	return c, r.Err()
}
