package action

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

// Reason says why a message could not be delivered.
type Reason uint8

// The reasons.
const (
	// Retired: the engine was removed.
	Retired Reason = iota + 1
	// NotBuilt: the engine does not exist yet, or the address names none.
	NotBuilt
	// Moved: what the message is about has gone to another engine.
	Moved
	// Stopping: the engine, or the world, is stopping.
	Stopping
	// Full: the inbox of the engine is full.
	Full
)

func (r Reason) String() string {
	switch r {
	case Retired:
		return "retired"
	case NotBuilt:
		return "not built"
	case Moved:
		return "moved"
	case Stopping:
		return "stopping"
	case Full:
		return "full"
	}
	return "unknown"
}

// Message is a message with its type taken away, which is what travels between the engines: the
// outbox, the router and the bounce table handle messages and do not know which kind of action
// one carries.
type Message interface {
	Dest() Address
	Head() Header
	// Subject is the actor that the message is about (the author of its first action that has
	// one): the one that is told when the message could not be delivered and the OnFail is to
	// notify. It is nil when no action of the message has an author.
	Subject() any
	OnFail() OnFail
	// encodeBody writes what EncodeMessage writes after the address.
	encodeBody(w *binarycodec.Writer) error
	Attempts() int
	ActionIDs() []ID
	// Stamp, Again and Redirected return copies.
	Stamp(Header) Message
	Again() Message
	Redirected(Address) Message
}

// ScopedMessage is a list of actions for one engine, that are put in its plan together, with what a
// failed delivery means. A message is delivered whole or bounced whole.
type ScopedMessage[A Action] struct {
	Header   Header
	To       Address
	Actions  []A
	onFail   OnFail
	attempts int
	subject  any
}

func newScopedMessage[A Action](to Address, onFail OnFail, actions []A) *ScopedMessage[A] {
	m := &ScopedMessage[A]{To: to, Actions: actions, onFail: onFail}
	for _, a := range actions {
		if who, ok := any(a).(Acting); ok {
			m.subject = who.Actor()
			break
		}
	}
	return m
}

// Dest, Head, Subject, OnFail and Attempts make a ScopedMessage a Message.
func (m *ScopedMessage[A]) Dest() Address  { return m.To }
func (m *ScopedMessage[A]) Head() Header   { return m.Header }
func (m *ScopedMessage[A]) Subject() any   { return m.subject }
func (m *ScopedMessage[A]) OnFail() OnFail { return m.onFail }
func (m *ScopedMessage[A]) Attempts() int  { return m.attempts }

// ActionIDs returns the IDs of the actions in the message.
func (m *ScopedMessage[A]) ActionIDs() []ID {
	ids := make([]ID, len(m.Actions))
	for i, a := range m.Actions {
		ids[i] = a.ID()
	}
	return ids
}

// Stamp returns a copy that carries the header.
func (m *ScopedMessage[A]) Stamp(h Header) Message {
	c := *m
	c.Header = h
	return &c
}

// Again returns a copy for one more attempt.
func (m *ScopedMessage[A]) Again() Message {
	c := *m
	c.attempts++
	return &c
}

// Redirected returns a copy for another engine of the same scope.
func (m *ScopedMessage[A]) Redirected(to Address) Message {
	c := *m
	c.To = to
	return &c
}

// MessageToZoneLayer makes a message for a zonelayer.
func MessageToZoneLayer(
	zoneLayerID state.ZoneLayerID, onFail OnFail, actions ...ZoneLayerAction,
) Message {
	return newScopedMessage(ZoneLayerAddress(zoneLayerID), onFail, actions)
}

// MessageToZone makes a message for a zone.
func MessageToZone(zoneID state.ZoneID, onFail OnFail, actions ...ZoneAction) Message {
	return newScopedMessage(ZoneAddress(zoneID), onFail, actions)
}

// MessageToWorld makes a message for a world.
func MessageToWorld(worldID state.WorldID, onFail OnFail, actions ...WorldAction) Message {
	return newScopedMessage(WorldAddress(worldID), onFail, actions)
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
