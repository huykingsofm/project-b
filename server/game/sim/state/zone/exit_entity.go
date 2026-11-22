package zone

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/shared/binarycodec"
)

// ExitEntity is an exit of a zone: an area of its map that leads to another zone. It stands in
// every zonelayer of the zone at once, so it belongs to the zone and not to one of its zonelayers.
type ExitEntity struct {
	entityID state.EntityID
	area     state.Rect // the tiles of the exit, both ends included
	toZoneID sim.StateID
	arrival  state.Vec // where a character comes out in the other zone
}

// ID returns the ID of the exit.
func (e ExitEntity) ID() state.EntityID { return e.entityID }

// Position is the middle of the area.
func (e ExitEntity) Position() state.Vec {
	return state.Vec{
		X: e.area.Min.X + (e.area.Max.X-e.area.Min.X)/2,
		Y: e.area.Min.Y + (e.area.Max.Y-e.area.Min.Y)/2,
	}
}

// Radius is half of the larger side of the area, so that a circle of it covers the area.
func (e ExitEntity) Radius() int32 {
	return max((e.area.Max.X-e.area.Min.X+1)/2, (e.area.Max.Y-e.area.Min.Y+1)/2)
}

// BoundingBox is the area.
func (e ExitEntity) BoundingBox() state.Rect { return e.area }

// Contains reports whether the point is in the area.
func (e ExitEntity) Contains(v state.Vec) bool { return e.area.Contains(v) }

// Hash adds what the exit is made of to the hash.
func (e ExitEntity) Hash(h *sim.Hasher) {
	h.U64(uint64(e.entityID))
	box := e.BoundingBox()
	for _, n := range [...]int32{box.Min.X, box.Min.Y, box.Max.X, box.Max.Y} {
		h.U64(uint64(uint32(n)))
	}
	h.U64(uint64(e.toZoneID))
	h.U64(uint64(uint32(e.arrival.X)))
	h.U64(uint64(uint32(e.arrival.Y)))
}

// Encode writes the exit.
func (e ExitEntity) Encode(w *binarycodec.Writer) {
	w.Uvarint(uint64(e.entityID))
	e.area.Min.Encode(w)
	e.area.Max.Encode(w)
	w.Uvarint(uint64(e.toZoneID))
	e.arrival.Encode(w)
}

// decodeExitEntity reads what Encode wrote, of the data version given.
func decodeExitEntity(r *binarycodec.Reader, version binarycodec.Version) (ExitEntity, error) {
	e := ExitEntity{entityID: state.EntityID(r.Uvarint())}
	e.area.Min = state.DecodeVec(r, version)
	e.area.Max = state.DecodeVec(r, version)
	e.toZoneID = sim.StateID(r.Uvarint())
	e.arrival = state.DecodeVec(r, version)
	return e, r.Err()
}

// Area is the exit, both ends included, in units.
func (e ExitEntity) Area() state.Rect { return e.area }

// ToZoneID is the ID of the zone the exit leads to.
func (e ExitEntity) ToZoneID() sim.StateID { return e.toZoneID }

// Arrival is where a character comes out in that zone, in units.
func (e ExitEntity) Arrival() state.Vec { return e.arrival }
