package command

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// ZoneLayerCharacterExitToCommand asks to take the exit, an entity of the zonelayer, that the
// character stands in.
type ZoneLayerCharacterExitToCommand struct {
	CharacterID  state.CharacterID
	ExitEntityID state.EntityID
}

func (ZoneLayerCharacterExitToCommand) ID() ID { return ZoneLayerCharacterExitToCommandID }

// Clone returns the command itself: it holds only values.
func (c ZoneLayerCharacterExitToCommand) Clone() ZoneLayerCommand { return c }

func (c ZoneLayerCharacterExitToCommand) TranslateZoneLayer() []action.ZoneLayerAction {
	return []action.ZoneLayerAction{
		&action.ZoneLayerCharacterExitToAction{
			CharacterID: c.CharacterID, ExitEntityID: c.ExitEntityID,
		},
	}
}

// Encode writes the fields of the command, not its ID: package command writes the ID first.
func (c ZoneLayerCharacterExitToCommand) Encode(w *binarycodec.Writer) error {
	w.Uvarint(uint64(c.CharacterID))
	w.Uvarint(uint64(c.ExitEntityID))
	return nil
}

// decodeZoneLayerCharacterExitToCommand reads what Encode wrote.
func decodeZoneLayerCharacterExitToCommand(
	r *binarycodec.Reader, version binarycodec.Version,
) (Command, error) {
	c := ZoneLayerCharacterExitToCommand{
		CharacterID: state.CharacterID(r.Uvarint()), ExitEntityID: state.EntityID(r.Uvarint()),
	}
	return c, r.Err()
}
