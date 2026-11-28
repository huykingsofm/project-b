package entity

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ItemEntity is an item lying on the ground of a zonelayer. It can be picked up for as long as it
// is in the state; what removes it when its time is up is ZoneLayerItemEntityRemoveAction.
type ItemEntity struct {
	// where it lies
	entityID state.EntityID
	pos      state.Vec
	rad      int32

	// what it is, how many of that kind lie together, and how many ticks it lies on the ground
	// before ZoneLayerItemEntityRemoveAction takes it off (0 means never; it starts again each
	// time the item is put on the ground)
	itemID state.ItemID
	count  uint32
	expiry uint64
}

// NewItemEntity makes a stack of count items of a kind on the ground, which is removed at the tick
// expiry (0 for never).
func NewItemEntity(
	id state.EntityID, pos state.Vec, itemID state.ItemID, count uint32, expiry uint64,
) ItemEntity {
	return ItemEntity{entityID: id, pos: pos, itemID: itemID, count: count, expiry: expiry}
}

func (e ItemEntity) ID() state.EntityID { return e.entityID }

func (e ItemEntity) KindID() state.KindID { return state.ItemKindID }

func (e ItemEntity) Position() state.Vec { return e.pos }

func (e ItemEntity) Radius() int32 { return e.rad }

func (e ItemEntity) BoundingBox() state.Rect {
	return state.BoundingBoxAround(e.pos, e.rad)
}

func (e ItemEntity) MovedTo(pos state.Vec) state.Entity {
	e.pos = pos
	return e
}

func (e ItemEntity) Hash(h *sim.Hasher) {
	state.HashEntity(h, e)
	h.U64(uint64(e.itemID))
	h.U64(uint64(e.count))
	h.U64(e.expiry)
}

func (e ItemEntity) Encode(w *binary.Writer) {
	w.Uvarint(uint64(e.entityID))
	e.pos.Encode(w)
	w.Varint(int64(e.rad))
	w.Uvarint(uint64(e.itemID))
	w.Uvarint(uint64(e.count))
	w.Uvarint(e.expiry)
}

// ItemID is the kind of item that lies on the ground.
func (e ItemEntity) ItemID() state.ItemID { return e.itemID }

// Count is how many items there are in the stack.
func (e ItemEntity) Count() uint32 { return e.count }

// Expiry is the tick at which the item is removed, or 0 for never.
func (e ItemEntity) Expiry() uint64 { return e.expiry }

func decodeItemEntity(r *binary.Reader, version codec.Version) (state.Entity, error) {
	e := ItemEntity{
		entityID: state.EntityID(r.Uvarint()),
		pos:      state.DecodeVec(r, version),
		rad:      r.Int[int32](),
		itemID:   r.Uint[state.ItemID](),
		count:    r.Uint[uint32](),
		expiry:   r.Uvarint(),
	}
	return e, r.Err()
}
