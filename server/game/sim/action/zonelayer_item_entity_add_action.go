package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/logging"
)

// ZoneLayerItemEntityAddAction puts a stack of items on the ground of the zonelayer and sets the
// removal of it (ZoneLayerItemEntityRemoveAction) for the end of its expiry. It is how an item
// comes to the ground anywhere, and how it comes back when it could not be taken: the world sends
// it when the inventory is full, and a pickup sends it to its own zonelayer when its message to the
// world cannot be delivered. The item keeps its kind, count, position and expiry, gets a new
// entity, and its expiry starts again.
type ZoneLayerItemEntityAddAction struct {
	ItemID state.ItemID
	Count  uint32
	At     state.Vec
	// Expiry is how long the item lies on the ground; 0 means for ever.
	Expiry uint64
}

func (a *ZoneLayerItemEntityAddAction) ID() sim.ActionID { return ZoneLayerItemEntityAddActionID }
func (a *ZoneLayerItemEntityAddAction) Priority() int16  { return PriorityResolve }
func (a *ZoneLayerItemEntityAddAction) Actor() any       { return nil }

// Apply puts the items on the ground and sets their removal.
func (a *ZoneLayerItemEntityAddAction) Apply(
	c *sim.Context, s *zonelayerstate.State, r *sim.Registry[*zonelayerstate.State],
	_ *sim.Plan[*zonelayerstate.State],
) {
	if a.Count == 0 {
		return
	}
	id, err := s.AddItemEntity(a.ItemID, a.Count, a.At, a.Expiry)
	if err != nil {
		logging.Error(
			"an item could not be put on the ground",
			"zonelayer", s.ID(), "item", a.ItemID, "err", err,
		)
		return
	}
	if a.Expiry > 0 {
		r.ToSelfAt(c.Tick+a.Expiry, &ZoneLayerItemEntityRemoveAction{EntityID: id})
	}
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a *ZoneLayerItemEntityAddAction) Encode(w *binary.Writer) {
	w.Uvarint(uint64(a.ItemID))
	w.Uvarint(uint64(a.Count))
	a.At.Encode(w)
	w.Uvarint(a.Expiry)
}

// decodeZoneLayerItemEntityAddAction reads what Encode wrote.
func decodeZoneLayerItemEntityAddAction(
	r *binary.Reader, version codec.Version,
) (*ZoneLayerItemEntityAddAction, error) {
	a := &ZoneLayerItemEntityAddAction{
		ItemID: r.Uint[state.ItemID](),
		Count:  r.Uint[uint32](),
		At:     state.DecodeVec(r, version),
		Expiry: r.Uvarint(),
	}
	return a, r.Err()
}
