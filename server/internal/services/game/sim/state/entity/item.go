package entity

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// ItemEntity is an item lying on the ground of a zonelayer. It can be picked up for as long as it is in
// the state; what removes it when its time is up is ZoneLayerItemEntityRemoveAction.
type ItemEntity struct {
	EntityID state.EntityID
	Pos      state.Vec
	Rad      int32
	// ItemID says what the item is.
	ItemID state.ItemID
	// Count is how many items of that kind lie together.
	Count uint32
	// Expiry is how many ticks the item lies on the ground before ZoneLayerItemEntityRemoveAction takes it
	// off; 0 means it is never taken off. It starts again each time the item is put on the ground.
	Expiry uint64
}

func (e ItemEntity) ID() state.EntityID  { return e.EntityID }
func (e ItemEntity) KindID() KindID      { return ItemKindID }
func (e ItemEntity) Position() state.Vec { return e.Pos }
func (e ItemEntity) Radius() int32       { return e.Rad }
func (e ItemEntity) BoundingBox() state.Rect {
	return boundingBoxAround(e.Pos, e.Rad)
}

func (e ItemEntity) MovedTo(pos state.Vec) Entity {
	e.Pos = pos
	return e
}

func (e ItemEntity) Hash(h *state.Hasher) {
	hashCommon(h, e)
	h.U64(uint64(e.ItemID))
	h.U64(uint64(e.Count))
	h.U64(e.Expiry)
}
