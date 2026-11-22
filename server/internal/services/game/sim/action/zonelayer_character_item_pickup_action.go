package action

import (
	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
)

var zoneLayerCharacterItemPickupActionRadius = config.NewVar("zonelayer.pickup.radius", 2000, config.Range(0, 32000))

// ZoneLayerCharacterItemPickupAction is a character picking up an item that lies on the ground of its
// zonelayer. The character chooses the exact entity of the item (the client works out which one it
// means), and must be within the pickup radius, in units, of the item: no part of its body farther than that from the item.
//
// The item is taken off the ground first, and only then is a message sent to the world to put it in the
// inventory of the character (R28), so the item is never in both places. What does not fit in the
// inventory comes back to the ground (WorldCharacterItemAddAction), and so does the whole stack when the
// message cannot be delivered, in both cases with the same kind, count, position and expiry, as a new entity whose expiry starts again.
type ZoneLayerCharacterItemPickupAction struct {
	CharacterID  state.CharacterID
	ItemEntityID state.EntityID
}

func (a *ZoneLayerCharacterItemPickupAction) ID() ID          { return ZoneLayerCharacterItemPickupActionID }
func (a *ZoneLayerCharacterItemPickupAction) Priority() int16 { return PriorityIntent }
func (a *ZoneLayerCharacterItemPickupAction) Actor() any      { return a.CharacterID }

// ApplyZoneLayer takes the item off the ground and sends it to the world.
func (a *ZoneLayerCharacterItemPickupAction) ApplyZoneLayer(c *Context, s *zonelayerstate.State, r *ZoneLayerRegistry, _ *Plan[ZoneLayerAction]) {
	ch, ok := s.CharacterByID(a.CharacterID)
	if !ok || ch.Health <= 0 {
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
	reach := item.BoundingBox().Grown(int32(zoneLayerCharacterItemPickupActionRadius.Get(c.Cfg)))
	if !reach.Intersects(body.BoundingBox()) {
		return
	}
	s.RemoveItemEntity(item.EntityID)
	// If the message cannot be delivered the item goes back to the ground as it was, by the same action
	// that puts an item on the ground anywhere.
	selfID := s.ID
	r.ToWorld(OnFailCompensate(func(Reason) Message {
		return MessageToZoneLayer(selfID, OnFailDrop, &ZoneLayerItemEntityAddAction{ItemID: item.ItemID, Count: item.Count, At: item.Pos, Expiry: item.Expiry})
	}),
		&WorldCharacterItemAddAction{CharacterID: a.CharacterID, ItemID: item.ItemID, Count: item.Count,
			FromZoneLayerID: selfID, At: item.Pos, Expiry: item.Expiry})
}
