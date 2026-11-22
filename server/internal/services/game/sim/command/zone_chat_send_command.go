package command

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// ZoneChatSendCommand asks to say something to the zone.
type ZoneChatSendCommand struct {
	FromID state.CharacterID
	Text   string
}

func (ZoneChatSendCommand) ID() ID { return ZoneChatSendCommandID }

// Clone returns the command itself: it holds only values.
func (c ZoneChatSendCommand) Clone() ZoneCommand { return c }

func (c ZoneChatSendCommand) TranslateZone() []action.ZoneAction {
	return []action.ZoneAction{&action.ZoneChatSendAction{FromID: c.FromID, Text: c.Text}}
}

// Encode writes the fields of the command, not its ID: package command writes the ID first.
func (c ZoneChatSendCommand) Encode(w *binarycodec.Writer) error {
	w.Uvarint(uint64(c.FromID))
	w.String(c.Text)
	return nil
}

// decodeZoneChatSendCommand reads what Encode wrote.
func decodeZoneChatSendCommand(
	r *binarycodec.Reader, version binarycodec.Version,
) (Command, error) {
	c := ZoneChatSendCommand{FromID: state.CharacterID(r.Uvarint()), Text: r.String()}
	return c, r.Err()
}
