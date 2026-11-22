package zonelayer

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/shared/binarycodec"
)

// ItemEntity is an item lying on the ground of a zonelayer. It can be picked up for as long as it
// is in the state; what removes it when its time is up is ZoneLayerItemEntityRemoveAction.
type ItemEntity struct {
	EntityID state.EntityID
	Pos      state.Vec
	Rad      int32
	// ItemID says what the item is.
	ItemID state.ItemID
	// Count is how many items of that kind lie together.
	Count uint32
	// Expiry is how many ticks the item lies on the ground before ZoneLayerItemEntityRemoveAction
	// takes it off; 0 means it is never taken off. It starts again each time the item is put on the
	// ground.
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

func (e ItemEntity) Hash(h *sim.Hasher) {
	hashCommon(h, e)
	h.U64(uint64(e.ItemID))
	h.U64(uint64(e.Count))
	h.U64(e.Expiry)
}

func (e ItemEntity) Encode(w *binarycodec.Writer) {
	w.Uvarint(uint64(e.EntityID))
	e.Pos.Encode(w)
	w.Varint(int64(e.Rad))
	w.Uvarint(uint64(e.ItemID))
	w.Uvarint(uint64(e.Count))
	w.Uvarint(e.Expiry)
}

func decodeItemEntity(r *binarycodec.Reader, version binarycodec.Version) (Entity, error) {
	e := ItemEntity{
		EntityID: state.EntityID(r.Uvarint()),
		Pos:      state.DecodeVec(r, version),
		Rad:      int32(r.Varint()),
		ItemID:   state.ItemID(r.Uvarint()),
		Count:    uint32(r.Uvarint()),
		Expiry:   r.Uvarint(),
	}
	return e, r.Err()
}
