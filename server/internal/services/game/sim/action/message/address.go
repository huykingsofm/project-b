package message

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// Scope is a kind of engine: the world, a zone or a zonelayer. (Level would clash with the level of
// a character.)
type Scope uint8

// The scopes.
const (
	ScopeWorld Scope = iota + 1
	ScopeZone
	ScopeZoneLayer
)

// String is the name of the scope, as the replay log names a directory.
func (s Scope) String() string {
	switch s {
	case ScopeWorld:
		return "world"
	case ScopeZone:
		return "zone"
	case ScopeZoneLayer:
		return "zonelayer"
	}
	return "unknown"
}

// Address says which engine a message is for: its scope, and its ID (the world ID, the zone ID or
// the zonelayer ID).
type Address struct {
	Scope Scope
	ID    uint32
}

// WorldAddress is the address of a world.
func WorldAddress(worldID state.WorldID) Address { return Address{ScopeWorld, uint32(worldID)} }

// ZoneAddress is the address of a zone.
func ZoneAddress(zoneID state.ZoneID) Address { return Address{ScopeZone, uint32(zoneID)} }

// ZoneLayerAddress is the address of a zonelayer.
func ZoneLayerAddress(zoneLayerID state.ZoneLayerID) Address {
	return Address{ScopeZoneLayer, uint32(zoneLayerID)}
}

// Header says who sent a message and which message of its sender it was, so that a replay can put
// the messages of an engine back in the order they came and a receiver can tell two messages apart.
type Header struct {
	From Address
	Seq  uint64
}

// Encode writes an address: its scope and its ID.
func (a Address) Encode(w *binarycodec.Writer) {
	w.Uint8(uint8(a.Scope))
	w.Uvarint(uint64(a.ID))
}

// DecodeAddress reads what Address.Encode wrote, of the data version given. A reader that has
// failed gives the zero address.
func DecodeAddress(r *binarycodec.Reader, version binarycodec.Version) Address {
	return Address{Scope: Scope(r.Uint8()), ID: uint32(r.Uvarint())}
}

// Encode writes a header: who sent the message and which message of the sender it was.
func (h Header) Encode(w *binarycodec.Writer) {
	h.From.Encode(w)
	w.Uvarint(h.Seq)
}

// DecodeHeader reads what Header.Encode wrote, of the data version given.
func DecodeHeader(r *binarycodec.Reader, version binarycodec.Version) Header {
	return Header{From: DecodeAddress(r, version), Seq: r.Uvarint()}
}
