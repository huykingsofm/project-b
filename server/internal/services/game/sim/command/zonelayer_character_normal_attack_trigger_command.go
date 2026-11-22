package command

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// ZoneLayerCharacterNormalAttackTriggerCommand asks to hit a target character with the normal
// attack.
type ZoneLayerCharacterNormalAttackTriggerCommand struct {
	CharacterID state.CharacterID
	TargetID    state.CharacterID
}

func (ZoneLayerCharacterNormalAttackTriggerCommand) ID() ID {
	return ZoneLayerCharacterNormalAttackTriggerCommandID
}

// Clone returns the command itself: it holds only values.
func (c ZoneLayerCharacterNormalAttackTriggerCommand) Clone() ZoneLayerCommand { return c }

func (
	c ZoneLayerCharacterNormalAttackTriggerCommand,
) TranslateZoneLayer() []action.ZoneLayerAction {
	return []action.ZoneLayerAction{
		&action.ZoneLayerCharacterNormalAttackTriggerAction{
			CharacterID: c.CharacterID, TargetID: c.TargetID,
		},
	}
}

// Encode writes the fields of the command, not its ID: package command writes the ID first.
func (c ZoneLayerCharacterNormalAttackTriggerCommand) Encode(w *binarycodec.Writer) error {
	w.Uvarint(uint64(c.CharacterID))
	w.Uvarint(uint64(c.TargetID))
	return nil
}

// decodeZoneLayerCharacterNormalAttackTriggerCommand reads what Encode wrote.
func decodeZoneLayerCharacterNormalAttackTriggerCommand(
	r *binarycodec.Reader, version binarycodec.Version,
) (Command, error) {
	c := ZoneLayerCharacterNormalAttackTriggerCommand{
		CharacterID: state.CharacterID(r.Uvarint()), TargetID: state.CharacterID(r.Uvarint()),
	}
	return c, r.Err()
}
