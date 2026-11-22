package command

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// ZoneLayerCharacterMoveCommand asks to hold a direction for a number of ticks.
type ZoneLayerCharacterMoveCommand struct {
	CharacterID state.CharacterID
	Dir         state.Dir
	Ticks       uint32
}

func (ZoneLayerCharacterMoveCommand) ID() ID { return ZoneLayerCharacterMoveCommandID }

func (c ZoneLayerCharacterMoveCommand) TranslateZoneLayer() []action.ZoneLayerAction {
	return []action.ZoneLayerAction{
		&action.ZoneLayerCharacterMoveAction{
			CharacterID: c.CharacterID, Dir: c.Dir, Ticks: c.Ticks,
		},
	}
}

// Encode writes the fields of the command, not its ID: package command writes the ID first.
func (c ZoneLayerCharacterMoveCommand) Encode(w *binarycodec.Writer) error {
	w.Uvarint(uint64(c.CharacterID))
	w.Uint8(uint8(c.Dir))
	w.Uvarint(uint64(c.Ticks))
	return nil
}

// decodeZoneLayerCharacterMoveCommand reads what Encode wrote.
func decodeZoneLayerCharacterMoveCommand(
	r *binarycodec.Reader, version binarycodec.Version,
) (Command, error) {
	c := ZoneLayerCharacterMoveCommand{
		CharacterID: state.CharacterID(r.Uvarint()),
		Dir:         state.Dir(r.Uint8()),
		Ticks:       uint32(r.Uvarint()),
	}
	return c, r.Err()
}
