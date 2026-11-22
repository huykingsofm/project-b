// Package message is what the engines and the actions say to one another: the address of an
// engine, the header of a message, why a delivery failed, and the interfaces of a message, of what
// a failed delivery means (OnFail) and of what that asks of the engine that sent it (Bouncer). The
// kinds of OnFail are in package onfail, and the messages that carry actions are made in package
// action.
package message

import (
	"errors"

	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// ErrNoCodec is returned for an action, a message or an OnFail that cannot be written or read back:
// its ID has no decoder, or it has no Encode.
var ErrNoCodec = errors.New("message: no codec for it")

// ErrUnknownCode is returned when bytes name an action, a scope or an OnFail that is not one.
var ErrUnknownCode = errors.New("message: the number is not one of an action, a scope or an OnFail")

// Message is a message with its type taken away, which is what travels between the engines: the
// outbox, the router and the bouncer handle messages and do not know which kind of action one
// carries.
type Message interface {
	Dest() Address
	Head() Header
	// Subject is the actor that the message is about (the author of its first action that has
	// one): the one that is told when the message could not be delivered and the OnFail is to
	// notify. It is nil when no action of the message has an author.
	Subject() any
	OnFail() OnFail
	Attempts() int
	// Stamp, Again and Redirected return copies.
	Stamp(Header) Message
	Again() Message
	Redirected(Address) Message
	// Encode writes the message whole: where it goes, who sent it, how many times it was tried, its
	// OnFail and its actions. Nothing of a message that cannot be written is left in w.
	Encode(w *binarycodec.Writer) error
}

// MessageDecoder reads a message that Message.Encode wrote, of the data version given. A kind of
// OnFail that holds a message is given one to read it with.
type MessageDecoder func(r *binarycodec.Reader, version binarycodec.Version) (Message, error)
