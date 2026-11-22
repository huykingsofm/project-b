package command

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// ZoneLayerCharacterItemPickupCommand asks to pick up an item that lies on the ground.
type ZoneLayerCharacterItemPickupCommand struct {
	CharacterID  state.CharacterID
	ItemEntityID state.EntityID
}

func (ZoneLayerCharacterItemPickupCommand) ID() ID { return ZoneLayerCharacterItemPickupCommandID }

// Clone returns the command itself: it holds only values.
func (c ZoneLayerCharacterItemPickupCommand) Clone() ZoneLayerCommand { return c }

func (c ZoneLayerCharacterItemPickupCommand) TranslateZoneLayer() []action.ZoneLayerAction {
	return []action.ZoneLayerAction{&action.ZoneLayerCharacterItemPickupAction{CharacterID: c.CharacterID, ItemEntityID: c.ItemEntityID}}
}

// Encode writes the fields of the command, not its ID: package command writes the ID first.
func (c ZoneLayerCharacterItemPickupCommand) Encode(w *binarycodec.Writer) error {
	w.Uvarint(uint64(c.CharacterID))
	w.Uvarint(uint64(c.ItemEntityID))
	return nil
}

// decodeZoneLayerCharacterItemPickupCommand reads what Encode wrote.
func decodeZoneLayerCharacterItemPickupCommand(r *binarycodec.Reader, version binarycodec.Version) (Command, error) {
	c := ZoneLayerCharacterItemPickupCommand{CharacterID: state.CharacterID(r.Uvarint()), ItemEntityID: state.EntityID(r.Uvarint())}
	return c, r.Err()
}
