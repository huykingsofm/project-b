package action

import (
	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

var zoneLayerCharacterItemPickupActionRadius = config.NewVar(
	"zonelayer.pickup.radius", 2000, config.Range(0, 32000),
)

// ZoneLayerCharacterItemPickupAction is a character picking up an item that lies on the ground of
// its zonelayer. The character chooses the exact entity of the item (the client works out which one
// it means), and must be within the pickup radius, in units, of the item: no part of its body
// farther than that from the item.
//
// The item is taken off the ground first, and only then is a message sent to the world to put it in
// the inventory of the character (R28), so the item is never in both places. What does not fit in
// the inventory comes back to the ground (WorldCharacterItemAddAction), and so does the whole stack
// when the message cannot be delivered, in both cases with the same kind, count, position and
// expiry, as a new entity whose expiry starts again.
type ZoneLayerCharacterItemPickupAction struct {
	CharacterID  state.CharacterID
	ItemEntityID state.EntityID
}

func (a ZoneLayerCharacterItemPickupAction) ID() sim.ActionID {
	return ZoneLayerCharacterItemPickupActionID
}
func (a ZoneLayerCharacterItemPickupAction) Priority() int16 { return PriorityIntent }
func (a ZoneLayerCharacterItemPickupAction) Actor() any      { return a.CharacterID }

// Clone returns the action itself: it holds only values.
func (a ZoneLayerCharacterItemPickupAction) Clone() sim.Action[*zonelayerstate.State] {
	return a
}

// Apply takes the item off the ground and sends it to the world.
func (a ZoneLayerCharacterItemPickupAction) Apply(
	c *sim.Context, s *zonelayerstate.State, r *sim.Registry[*zonelayerstate.State],
	_ *sim.Plan[*zonelayerstate.State],
) {
	ch, ok := s.CharacterByID(a.CharacterID)
	if !ok || ch.Health() <= 0 {
		return
	}
	body, ok := s.CharacterEntityByCharacterID(a.CharacterID)
	if !ok {
		return
	}
	item, ok := s.ItemEntityByID(a.ItemEntityID)
	if !ok {
		return
	}
	reach := item.BoundingBox().Grown(
		int32(zoneLayerCharacterItemPickupActionRadius.Get(r.Config())),
	)
	if !reach.Intersects(body.BoundingBox()) {
		return
	}
	s.RemoveItemEntity(item.ID())
	// If the message cannot be delivered the item goes back to the ground as it was, by the same
	// action that puts an item on the ground anywhere.
	selfID := c.Self
	undo := sim.NewActionMessage(
		selfID,
		onfail.Drop{},
		ZoneLayerItemEntityAddAction{
			ItemID: item.ItemID(), Count: item.Count(), At: item.Position(), Expiry: item.Expiry(),
		},
	)
	r.Send(
		s.WorldID(), onfail.Compensate{With: undo},
		WorldCharacterItemAddAction{
			CharacterID: a.CharacterID, ItemID: item.ItemID(), Count: item.Count(),
			FromZoneLayerID: selfID, At: item.Position(), Expiry: item.Expiry(),
		},
	)
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a ZoneLayerCharacterItemPickupAction) Encode(w *binary.Writer) {
	w.Uvarint(uint64(a.CharacterID))
	a.ItemEntityID.Encode(w)
}

// decodeZoneLayerCharacterItemPickupAction reads what Encode wrote.
func decodeZoneLayerCharacterItemPickupAction(
	r *binary.Reader, version codec.Version,
) (ZoneLayerCharacterItemPickupAction, error) {
	characterID := r.Uint[state.CharacterID]()
	itemEntityID, err := state.DecodeEntityID(r)
	if err != nil {
		return ZoneLayerCharacterItemPickupAction{}, err
	}
	return ZoneLayerCharacterItemPickupAction{
		CharacterID: characterID, ItemEntityID: itemEntityID,
	}, r.Err()
}
