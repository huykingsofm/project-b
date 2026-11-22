package zone

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/shared/binarycodec"
)

// ExitEntity is an exit of a zone: an area of its map that leads to another zone. It stands in
// every zonelayer of the zone at once, so it belongs to the zone and not to one of its zonelayers.
type ExitEntity struct {
	EntityID state.EntityID
	Area     state.Rect // the tiles of the exit, both ends included
	ToZoneID sim.StateID
	Arrival  state.Vec // where a character comes out in the other zone
}

// ID returns the ID of the exit.
func (e ExitEntity) ID() state.EntityID { return e.EntityID }

// Position is the middle of the area.
func (e ExitEntity) Position() state.Vec {
	return state.Vec{
		X: e.Area.Min.X + (e.Area.Max.X-e.Area.Min.X)/2,
		Y: e.Area.Min.Y + (e.Area.Max.Y-e.Area.Min.Y)/2,
	}
}

// Radius is half of the larger side of the area, so that a circle of it covers the area.
func (e ExitEntity) Radius() int32 {
	return max((e.Area.Max.X-e.Area.Min.X+1)/2, (e.Area.Max.Y-e.Area.Min.Y+1)/2)
}

// BoundingBox is the area.
func (e ExitEntity) BoundingBox() state.Rect { return e.Area }

// Contains reports whether the point is in the area.
func (e ExitEntity) Contains(v state.Vec) bool { return e.Area.Contains(v) }

// Hash adds what the exit is made of to the hash.
func (e ExitEntity) Hash(h *sim.Hasher) {
	h.U64(uint64(e.EntityID))
	box := e.BoundingBox()
	for _, n := range [...]int32{box.Min.X, box.Min.Y, box.Max.X, box.Max.Y} {
		h.U64(uint64(uint32(n)))
	}
	h.U64(uint64(e.ToZoneID))
	h.U64(uint64(uint32(e.Arrival.X)))
	h.U64(uint64(uint32(e.Arrival.Y)))
}

// Encode writes the exit.
func (e ExitEntity) Encode(w *binarycodec.Writer) {
	w.Uvarint(uint64(e.EntityID))
	e.Area.Min.Encode(w)
	e.Area.Max.Encode(w)
	w.Uvarint(uint64(e.ToZoneID))
	e.Arrival.Encode(w)
}

// decodeExitEntity reads what Encode wrote, of the data version given.
func decodeExitEntity(r *binarycodec.Reader, version binarycodec.Version) (ExitEntity, error) {
	e := ExitEntity{EntityID: state.EntityID(r.Uvarint())}
	e.Area.Min = state.DecodeVec(r, version)
	e.Area.Max = state.DecodeVec(r, version)
	e.ToZoneID = sim.StateID(r.Uvarint())
	e.Arrival = state.DecodeVec(r, version)
	return e, r.Err()
}
