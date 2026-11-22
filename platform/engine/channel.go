package engine

import (
	"github.com/huykingsofm/project-b/platform/sim"
)

// ReturnedMessage is a message that came back to its sender because it could not be delivered.
type ReturnedMessage struct {
	Message sim.Message
	Reason  sim.Reason
}

// IncomingChannel is the receiving end of the two channels of an engine: In for the messages that
// other engines send it, and Back for the messages of its own that came back undelivered. Only the
// engine itself holds it, and it can only read.
type IncomingChannel struct {
	In   <-chan sim.Message
	Back <-chan ReturnedMessage
}

// OutgoingChannel is the sending end of the same two channels. The router holds it, to carry
// messages to the engine: it can only write, and it never waits, because a full channel refuses.
type OutgoingChannel struct {
	id   sim.StateID
	in   chan<- sim.Message
	back chan<- ReturnedMessage
}

// NewChannel makes the two channels of an engine, with room for size messages each, and returns the
// end that the engine reads and the end that is given to the router.
func NewChannel(id sim.StateID, size int) (IncomingChannel, OutgoingChannel) {
	in := make(chan sim.Message, size)
	back := make(chan ReturnedMessage, size)
	return IncomingChannel{In: in, Back: back}, OutgoingChannel{
		id: id, in: in, back: back,
	}
}

// StateID returns the ID of the engine that the channel leads to.
func (o OutgoingChannel) StateID() sim.StateID { return o.id }

// Send puts a message in the In channel of the engine. It reports false when the channel is full.
func (o OutgoingChannel) Send(m sim.Message) bool {
	select {
	case o.in <- m:
		return true
	default:
		return false
	}
}

// SendBack puts a returned message in the Back channel of the engine. It reports false when the
// channel is full.
func (o OutgoingChannel) SendBack(r ReturnedMessage) bool {
	select {
	case o.back <- r:
		return true
	default:
		return false
	}
}
