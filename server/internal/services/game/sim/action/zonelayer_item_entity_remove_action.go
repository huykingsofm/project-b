package action

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
)

// ZoneLayerItemEntityRemoveAction takes an item off the ground when its time is up. It is set by
// ZoneLayerItemEntityAddAction. An item that was picked up in the meantime is no longer in the
// state, and an entity ID is never used twice in a zonelayer, so then it does nothing.
type ZoneLayerItemEntityRemoveAction struct {
	EntityID state.EntityID
}

func (a *ZoneLayerItemEntityRemoveAction) ID() ID {
	return ZoneLayerItemEntityRemoveActionID
}
func (a *ZoneLayerItemEntityRemoveAction) Priority() int16 { return PriorityResolve }

// ApplyZoneLayer removes the item if it is still on the ground.
func (a *ZoneLayerItemEntityRemoveAction) ApplyZoneLayer(
	_ *Context, s *zonelayerstate.State, _ *ZoneLayerRegistry, _ *Plan[ZoneLayerAction],
) {
	s.RemoveItemEntity(a.EntityID)
}
