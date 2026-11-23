package command

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/binarycodec"
)

// ZoneLayerCharacterItemPickupCommand asks to pick up an item that lies on the ground.
type ZoneLayerCharacterItemPickupCommand struct {
	CharacterID  state.CharacterID
	ItemEntityID state.EntityID
}

func (ZoneLayerCharacterItemPickupCommand) ID() sim.CommandID {
	return ZoneLayerCharacterItemPickupCommandID
}

// Clone returns the command itself: it holds only values.
func (c ZoneLayerCharacterItemPickupCommand) Clone() sim.Command[*zonelayerstate.State] { return c }

func (c ZoneLayerCharacterItemPickupCommand) Translate() []sim.Action[*zonelayerstate.State] {
	return []sim.Action[*zonelayerstate.State]{
		&action.ZoneLayerCharacterItemPickupAction{
			CharacterID: c.CharacterID, ItemEntityID: c.ItemEntityID,
		},
	}
}

// Encode writes the fields of the command, not its ID: sim.EncodeCommand writes the ID first.
func (c ZoneLayerCharacterItemPickupCommand) Encode(w *binarycodec.Writer) error {
	w.Uvarint(uint64(c.CharacterID))
	w.Uvarint(uint64(c.ItemEntityID))
	return nil
}

// decodeZoneLayerCharacterItemPickupCommand reads what Encode wrote.
func decodeZoneLayerCharacterItemPickupCommand(
	r *binarycodec.Reader, version binarycodec.Version,
) (sim.Command[*zonelayerstate.State], error) {
	c := ZoneLayerCharacterItemPickupCommand{
		CharacterID: r.Uint[state.CharacterID](), ItemEntityID: r.Uint[state.EntityID](),
	}
	return c, r.Err()
}
