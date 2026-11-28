package sim

import (
	"errors"

	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ErrNoCodec is returned for an action, a command, a message or an OnFail that cannot be written
// or read back: its ID has no decoder, or it has no Encode.
var ErrNoCodec = errors.New("sim: no codec for it")

// ErrUnknownCode is returned when bytes name an action, a command, a state type or an OnFail that
// is not one.
var ErrUnknownCode = errors.New(
	"sim: the number is not one of an action, a command, a state or an OnFail",
)

// The reasons.
const (
	// DeliveryFailureRetired: the engine was retired.
	DeliveryFailureRetired DeliveryFailure = iota + 1
	// DeliveryFailureNotStarted: the engine does not exist yet, or the ID names none.
	DeliveryFailureNotStarted
	// DeliveryFailureStopping: the engine, or the cluster, is stopping.
	DeliveryFailureStopping
	// DeliveryFailureFull: the inbox of the engine is full.
	DeliveryFailureFull
)

// Bouncer is what an OnFail may ask of the engine that sent a message that could not be delivered.
type Bouncer interface {
	// Resend makes the engine send the message at its next flush.
	Resend(m Message)
	// Drop forgets a message and counts it.
	Drop()
	// Stopping reports whether the engine is stopping, when nothing is sent again.
	Stopping() bool
	// MaxAttempts is how many times a message is tried before it is dropped.
	MaxAttempts() int
}

// Message is a message with its type taken away, which is what travels between the engines: the
// outbox, the router and the bouncer handle messages and do not know which type of state one is
// for.
type Message interface {
	// Dest is the ID of the state, and so of the engine, that the message is for.
	Dest() StateID
	Head() Header
	OnFail() OnFail
	Attempts() int
	// Stamp, Again and Redirected return copies.
	Stamp(Header) Message
	Again() Message
	Redirected(to StateID) Message
	// Encode writes the message whole: where it goes (which says the type of state it is for), who
	// sent it, how many times it was tried, its OnFail and its actions. Nothing of a message that
	// cannot be written is left in w.
	Encode(w *binary.Writer) error
	// Clone returns a copy that shares nothing with the message: its actions and its OnFail are
	// copied with their Clone. Stamp, Again and Redirected copy the message and share them.
	Clone() Message
}

// OnFailID names a kind of OnFail, by a number: see the register in package onfail. A kind returns
// its own with its method ID(). A number is never changed or reused, because the event log writes
// it.
type OnFailID uint16

// OnFail is the part of a message that says what a failed delivery means. Each kind is a type of
// its own that holds what it needs, runs itself on the engine that sent the message (Handle), and
// writes itself (Encode); a function in its codec reads it back, found by its ID. A message is
// written whole, so an OnFail is data and never a function. Every send names one: a message with
// none is dropped, and logged.
type OnFail interface {
	ID() OnFailID
	// Handle does what a failed delivery means, given the engine that sent the message, the
	// message and why it could not be delivered.
	Handle(b Bouncer, m Message, r DeliveryFailure)
	// Encode writes what the kind holds, not its ID: Codecs.EncodeOnFail writes the ID first.
	Encode(w *binary.Writer) error
	// Clone returns a copy that shares nothing with the kind: a kind that holds only values
	// returns itself.
	Clone() OnFail
}

// DeliveryFailure says why a message could not be delivered.
type DeliveryFailure uint8

func (r DeliveryFailure) String() string {
	switch r {
	case DeliveryFailureRetired:
		return "retired"
	case DeliveryFailureNotStarted:
		return "not started"
	case DeliveryFailureStopping:
		return "stopping"
	case DeliveryFailureFull:
		return "full"
	}
	return "unknown"
}

// Header says who sent a message, which message of its sender it was, and at which tick of its
// sender it was sent. The first two let a replay put the messages of an engine back in the order
// they came and a receiver tell two messages apart; the tick says which tick of the sender the
// receiver depends on once it takes the message (see StateDependency).
type Header struct {
	From StateID
	Seq  uint64
	Tick uint64 // the tick of the sender at which it sent the message; 0 for one that was not sent
}

// DecodeHeader reads what Header.Encode wrote.
func DecodeHeader(r *binary.Reader) Header {
	return Header{From: r.Uint[StateID](), Seq: r.Uvarint(), Tick: r.Uvarint()}
}

// Encode writes a header.
func (h Header) Encode(w *binary.Writer) {
	w.Uvarint(uint64(h.From))
	w.Uvarint(h.Seq)
	w.Uvarint(h.Tick)
}

// MessageDecoder reads a message that Message.Encode wrote, of the data version given. A kind of
// OnFail that holds a message is given one to read it with.
type MessageDecoder func(r *binary.Reader, version codec.Version) (Message, error)

// NestedOnFail is implemented by a kind of OnFail that holds a message, so that the depth of
// messages inside messages can be checked before one is written.
type NestedOnFail interface {
	Nested() Message
}
