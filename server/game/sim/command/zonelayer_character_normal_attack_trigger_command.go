package command

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ZoneLayerCharacterNormalAttackTriggerCommand asks to hit a target character with the normal
// attack.
type ZoneLayerCharacterNormalAttackTriggerCommand struct {
	CharacterID state.CharacterID
	TargetID    state.CharacterID
}

func (ZoneLayerCharacterNormalAttackTriggerCommand) ID() sim.CommandID {
	return ZoneLayerCharacterNormalAttackTriggerCommandID
}

// Clone returns the command itself: it holds only values.
func (c ZoneLayerCharacterNormalAttackTriggerCommand) Clone() sim.Command[*zonelayerstate.State] {
	return c
}

func (
	c ZoneLayerCharacterNormalAttackTriggerCommand,
) Translate() []sim.Action[*zonelayerstate.State] {
	return []sim.Action[*zonelayerstate.State]{
		&action.ZoneLayerCharacterNormalAttackTriggerAction{
			CharacterID: c.CharacterID, TargetID: c.TargetID,
		},
	}
}

// Encode writes the fields of the command, not its ID: sim.EncodeCommand writes the ID first.
func (c ZoneLayerCharacterNormalAttackTriggerCommand) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(c.CharacterID))
	w.Uvarint(uint64(c.TargetID))
	return nil
}

// decodeZoneLayerCharacterNormalAttackTriggerCommand reads what Encode wrote.
func decodeZoneLayerCharacterNormalAttackTriggerCommand(
	r *binary.Reader, version codec.Version,
) (sim.Command[*zonelayerstate.State], error) {
	c := ZoneLayerCharacterNormalAttackTriggerCommand{
		CharacterID: r.Uint[state.CharacterID](), TargetID: r.Uint[state.CharacterID](),
	}
	return c, r.Err()
}
