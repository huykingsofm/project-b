package action

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
)

// ZoneLayerItemEntityAddAction puts a stack of items on the ground of the zonelayer and sets the removal
// of it (ZoneLayerItemEntityRemoveAction) for the end of its expiry. It is how an item comes to the
// ground anywhere, and how it comes back when it could not be taken: the world sends it when the inventory
// is full, and a pickup sends it to its own zonelayer when its message to the world cannot be delivered.
// The item keeps its kind, count, position and expiry, gets a new entity, and its expiry starts again.
type ZoneLayerItemEntityAddAction struct {
	ItemID state.ItemID
	Count  uint32
	At     state.Vec
	// Expiry is how long the item lies on the ground; 0 means for ever.
	Expiry uint64
}

func (a *ZoneLayerItemEntityAddAction) ID() ID          { return ZoneLayerItemEntityAddActionID }
func (a *ZoneLayerItemEntityAddAction) Priority() int16 { return PriorityResolve }

// ApplyZoneLayer puts the items on the ground and sets their removal.
func (a *ZoneLayerItemEntityAddAction) ApplyZoneLayer(c *Context, s *zonelayerstate.State, r *ZoneLayerRegistry, _ *Plan[ZoneLayerAction]) {
	if a.Count == 0 {
		return
	}
	id := s.AddItemEntity(a.ItemID, a.Count, a.At, a.Expiry)
	if a.Expiry > 0 {
		r.ToSelfAt(c.Tick+a.Expiry, &ZoneLayerItemEntityRemoveAction{EntityID: id})
	}
}
