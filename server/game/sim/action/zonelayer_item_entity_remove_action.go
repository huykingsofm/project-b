package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ZoneLayerItemEntityRemoveAction takes an item off the ground when its time is up. It is set by
// ZoneLayerItemEntityAddAction. An item that was picked up in the meantime is no longer in the
// state, and an entity ID is never used twice in a zonelayer, so then it does nothing.
type ZoneLayerItemEntityRemoveAction struct {
	EntityID state.EntityID
}

func (a *ZoneLayerItemEntityRemoveAction) ID() sim.ActionID {
	return ZoneLayerItemEntityRemoveActionID
}
func (a *ZoneLayerItemEntityRemoveAction) Priority() int16 { return PriorityResolve }
func (a *ZoneLayerItemEntityRemoveAction) Actor() any      { return nil }

// Clone returns a copy that shares nothing with the action.
func (a *ZoneLayerItemEntityRemoveAction) Clone() sim.Action[*zonelayerstate.State] {
	c := *a
	return &c
}

// Apply removes the item if it is still on the ground.
func (a *ZoneLayerItemEntityRemoveAction) Apply(
	_ *sim.Context, s *zonelayerstate.State, _ *sim.Registry[*zonelayerstate.State],
	_ *sim.Plan[*zonelayerstate.State],
) {
	s.RemoveItemEntity(a.EntityID)
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a *ZoneLayerItemEntityRemoveAction) Encode(w *binary.Writer) {
	a.EntityID.Encode(w)
}

// decodeZoneLayerItemEntityRemoveAction reads what Encode wrote.
func decodeZoneLayerItemEntityRemoveAction(
	r *binary.Reader, version codec.Version,
) (*ZoneLayerItemEntityRemoveAction, error) {
	entityID, err := state.DecodeEntityID(r)
	if err != nil {
		return nil, err
	}
	return &ZoneLayerItemEntityRemoveAction{EntityID: entityID}, nil
}
