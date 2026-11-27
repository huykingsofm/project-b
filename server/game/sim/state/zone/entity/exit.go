package entity

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
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

// KindID is the kind of an exit.
func (ExitEntity) KindID() state.KindID { return state.ExitKindID }

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
	state.HashEntity(h, e)
	h.U64(uint64(e.toZoneID))
	h.U64(uint64(uint32(e.arrival.X)))
	h.U64(uint64(uint32(e.arrival.Y)))
}

// MovedTo returns the exit with its whole area moved so that its position is pos; the arrival
// stays.
func (e ExitEntity) MovedTo(pos state.Vec) state.Entity {
	at := e.Position()
	by := state.Vec{X: pos.X - at.X, Y: pos.Y - at.Y}
	e.area = state.Rect{Min: e.area.Min.Add(by), Max: e.area.Max.Add(by)}
	return e
}

// Encode writes the exit.
func (e ExitEntity) Encode(w *binary.Writer) {
	w.Uvarint(uint64(e.entityID))
	e.area.Min.Encode(w)
	e.area.Max.Encode(w)
	w.Uvarint(uint64(e.toZoneID))
	e.arrival.Encode(w)
}

// decodeExitEntity reads what Encode wrote, of the data version given.
func decodeExitEntity(r *binary.Reader, version codec.Version) (state.Entity, error) {
	e := ExitEntity{entityID: state.EntityID(r.Uvarint())}
	e.area.Min = state.DecodeVec(r, version)
	e.area.Max = state.DecodeVec(r, version)
	e.toZoneID = r.Uint[sim.StateID]()
	e.arrival = state.DecodeVec(r, version)
	return e, r.Err()
}

// Area is the exit, both ends included, in units.
func (e ExitEntity) Area() state.Rect { return e.area }

// ToZoneID is the ID of the zone the exit leads to.
func (e ExitEntity) ToZoneID() sim.StateID { return e.toZoneID }

// Arrival is where a character comes out in that zone, in units.
func (e ExitEntity) Arrival() state.Vec { return e.arrival }

// NewExitEntity makes an exit: the area that leads to the zone toZoneID, and where a character
// comes out there.
func NewExitEntity(
	id state.EntityID, area state.Rect, toZoneID sim.StateID, arrival state.Vec,
) ExitEntity {
	return ExitEntity{entityID: id, area: area, toZoneID: toZoneID, arrival: arrival}
}
