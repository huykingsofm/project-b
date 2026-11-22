package entity

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// ExitEntity is an exit of a map: an area that leads to a place in another zone. Every zonelayer of
// a zone has the exits of the map as entities.
type ExitEntity struct {
	EntityID state.EntityID
	Area     state.Rect // the tiles of the exit, both ends included
	ToZoneID state.ZoneID
	Arrival  state.Vec // where a character comes out in the other zone
}

func (e ExitEntity) ID() state.EntityID { return e.EntityID }
func (e ExitEntity) KindID() KindID     { return ExitKindID }

// Position is the middle of the area, rounded towards its Min.
func (e ExitEntity) Position() state.Vec {
	return state.Vec{
		X: e.Area.Min.X + (e.Area.Max.X-e.Area.Min.X)/2,
		Y: e.Area.Min.Y + (e.Area.Max.Y-e.Area.Min.Y)/2,
	}
}

// Radius is how far the area reaches from its position, the longer way.
func (e ExitEntity) Radius() int32 {
	return max((e.Area.Max.X-e.Area.Min.X+1)/2, (e.Area.Max.Y-e.Area.Min.Y+1)/2)
}

func (e ExitEntity) BoundingBox() state.Rect { return e.Area }

// Contains reports whether the position is inside the exit.
func (e ExitEntity) Contains(v state.Vec) bool { return e.Area.Contains(v) }

// MovedTo moves the whole area, and the arrival stays where it is.
func (e ExitEntity) MovedTo(pos state.Vec) Entity {
	by := state.Vec{X: pos.X - e.Position().X, Y: pos.Y - e.Position().Y}
	e.Area = state.Rect{Min: e.Area.Min.Add(by), Max: e.Area.Max.Add(by)}
	return e
}

func (e ExitEntity) Hash(h *state.Hasher) {
	hashCommon(h, e)
	h.U64(uint64(e.ToZoneID))
	h.U64(uint64(uint32(e.Arrival.X)))
	h.U64(uint64(uint32(e.Arrival.Y)))
}

func (e ExitEntity) Encode(w *binarycodec.Writer) {
	w.Uvarint(uint64(e.EntityID))
	e.Area.Min.Encode(w)
	e.Area.Max.Encode(w)
	w.Uvarint(uint64(e.ToZoneID))
	e.Arrival.Encode(w)
}

func decodeExitEntity(r *binarycodec.Reader, version binarycodec.Version) (Entity, error) {
	e := ExitEntity{EntityID: state.EntityID(r.Uvarint())}
	e.Area.Min = state.DecodeVec(r, version)
	e.Area.Max = state.DecodeVec(r, version)
	e.ToZoneID = state.ZoneID(r.Uvarint())
	e.Arrival = state.DecodeVec(r, version)
	return e, r.Err()
}
