package message

import "github.com/huykingsofm/project-b/server/internal/shared/binarycodec"

// OnFailID names a kind of OnFail, by a number: see the register in package onfail. A kind returns
// its own with its method ID(). A number is never changed or reused, because the replay log writes
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
	Handle(b Bouncer, m Message, r Reason)
	// Encode writes what the kind holds, not its ID: onfail.Encode writes the ID first.
	Encode(w *binarycodec.Writer) error
}

// Bouncer is what an OnFail may ask of the engine that sent a message that could not be delivered.
type Bouncer interface {
	// Resend makes the engine send the message at its next flush.
	Resend(m Message)
	// Tell tells an actor something, as a text key with arguments. An actor that is nil is told
	// nothing.
	Tell(actor any, key string, args []string)
	// Drop forgets a message and counts it.
	Drop()
	// Stopping reports whether the engine is stopping, when nothing is sent again.
	Stopping() bool
	// MaxAttempts is how many times a message is tried before it is dropped.
	MaxAttempts() int
}
