package command

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
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
func (c ZoneLayerCharacterItemPickupCommand) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(c.CharacterID))
	c.ItemEntityID.Encode(w)
	return nil
}

// decodeZoneLayerCharacterItemPickupCommand reads what Encode wrote.
func decodeZoneLayerCharacterItemPickupCommand(
	r *binary.Reader, version codec.Version,
) (ZoneLayerCharacterItemPickupCommand, error) {
	characterID := r.Uint[state.CharacterID]()
	itemEntityID, err := state.DecodeEntityID(r)
	if err != nil {
		return ZoneLayerCharacterItemPickupCommand{}, err
	}
	return ZoneLayerCharacterItemPickupCommand{
		CharacterID: characterID, ItemEntityID: itemEntityID,
	}, r.Err()
}
