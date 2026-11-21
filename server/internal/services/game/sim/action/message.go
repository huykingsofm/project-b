package action

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// Scope is a kind of engine: the world, a zone or a zonelayer. (Level would clash with the level of a
// character.)
type Scope uint8

// The scopes.
const (
	ScopeWorld Scope = iota + 1
	ScopeZone
	ScopeZoneLayer
)

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

// DecisionKind is what to do with a message that could not be delivered.
type DecisionKind uint8

// The kinds of decision. The engine that sent the message runs them from a table.
const (
	// DecisionRetry sends it again to the same address.
	DecisionRetry DecisionKind = iota + 1
	// DecisionRedirect sends it to another engine of the same scope.
	DecisionRedirect
	// DecisionCompensate sends another message that undoes what the first one set going.
	DecisionCompensate
	// DecisionNotify tells the actor that the message is about, and drops it.
	DecisionNotify
	// DecisionEscalate gives it to another engine to decide.
	DecisionEscalate
	// DecisionDrop forgets it and counts it.
	DecisionDrop
)

// Decision is what a message says to do when it could not be delivered. It is made by the
// OnFail functions below.
type Decision struct {
	Kind DecisionKind
	To   Address
	With Message
	Key  string
	Args []any
}

// OnFail is the part of a message that says what a failed delivery means: from the reason it gives
// the decision. Every send names one, so a send that makes no choice does not compile.
type OnFail func(Reason) Decision

// OnFailRetry sends the message again.
func OnFailRetry(Reason) Decision { return Decision{Kind: DecisionRetry} }

// OnFailDrop forgets the message.
func OnFailDrop(Reason) Decision { return Decision{Kind: DecisionDrop} }

// OnFailRedirect sends the message to the engine that the function chooses from the reason.
func OnFailRedirect(to func(Reason) Address) OnFail {
	return func(r Reason) Decision { return Decision{Kind: DecisionRedirect, To: to(r)} }
}

// OnFailCompensate sends the message that undo makes from the reason.
func OnFailCompensate(undo func(Reason) Message) OnFail {
	return func(r Reason) Decision { return Decision{Kind: DecisionCompensate, With: undo(r)} }
}

// OnFailNotify tells the actor that the message is about, with a text key.
func OnFailNotify(key string, args ...any) OnFail {
	return func(Reason) Decision { return Decision{Kind: DecisionNotify, Key: key, Args: args} }
}

// OnFailEscalate gives the message to the engine at the address.
func OnFailEscalate(to Address) OnFail {
	return func(Reason) Decision { return Decision{Kind: DecisionEscalate, To: to} }
}

// Message is a message with its type taken away, which is what travels between the engines: the
// outbox, the router and the bounce table handle messages and do not know which kind of action
// one carries.
type Message interface {
	Dest() Address
	Head() Header
	// Subject is the actor that the message is about (the author of its first action that has
	// one): the one that is told when the message could not be delivered and the decision is to
	// notify. It is nil when no action of the message has an author.
	Subject() any
	Bounce(Reason) Decision
	Attempts() int
	ActionIDs() []ID
	// Stamp, Again and Redirected return copies.
	Stamp(Header) Message
	Again() Message
	Redirected(Address) Message
}

// ScopedMessage is a list of actions for one engine, that are put in its plan together, with what a failed
// delivery means. A message is delivered whole or bounced whole.
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

// Dest, Head, Subject, Bounce and Attempts make a ScopedMessage a Message.
func (m *ScopedMessage[A]) Dest() Address            { return m.To }
func (m *ScopedMessage[A]) Head() Header             { return m.Header }
func (m *ScopedMessage[A]) Subject() any             { return m.subject }
func (m *ScopedMessage[A]) Bounce(r Reason) Decision { return m.onFail(r) }
func (m *ScopedMessage[A]) Attempts() int            { return m.attempts }

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
func MessageToZoneLayer(zoneLayerID state.ZoneLayerID, onFail OnFail, actions ...ZoneLayerAction) Message {
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
