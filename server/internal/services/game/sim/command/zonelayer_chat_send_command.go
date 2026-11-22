package command

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// ZoneLayerChatSendCommand asks to say something to the zonelayer the character is in.
type ZoneLayerChatSendCommand struct {
	FromID state.CharacterID
	Text   string
}

func (ZoneLayerChatSendCommand) ID() ID { return ZoneLayerChatSendCommandID }

func (c ZoneLayerChatSendCommand) TranslateZoneLayer() []action.ZoneLayerAction {
	return []action.ZoneLayerAction{&action.ZoneLayerChatSendAction{FromID: c.FromID, Text: c.Text}}
}

// Encode writes the fields of the command, not its ID: package command writes the ID first.
func (c ZoneLayerChatSendCommand) Encode(w *binarycodec.Writer) error {
	w.Uvarint(uint64(c.FromID))
	w.String(c.Text)
	return nil
}

// decodeZoneLayerChatSendCommand reads what Encode wrote.
func decodeZoneLayerChatSendCommand(
	r *binarycodec.Reader, version binarycodec.Version,
) (Command, error) {
	c := ZoneLayerChatSendCommand{FromID: state.CharacterID(r.Uvarint()), Text: r.String()}
	return c, r.Err()
}
