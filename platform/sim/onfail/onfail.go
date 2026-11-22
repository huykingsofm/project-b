// Package onfail holds the kinds of OnFail: what a message says to do when it could not be
// delivered. Each kind is a type that holds only what it needs, runs itself on the engine that sent
// the message through sim.Bouncer, and writes itself; its decoder is found by its ID (Decode).
// Each kind is in a file of its own.
package onfail

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/logging"
)

// The IDs of the kinds: the register. The numbers count up from 1 and 0 is no kind. A new kind
// takes the next number, at the end of the list, and a number is never changed or reused, because
// the replay log writes it.
const (
	RetryID sim.OnFailID = iota + 1
	RedirectID
	CompensateID
	NotifyID
	EscalateID
	DropID
)

// resendable reports whether a message that is to be sent again may be: not while the engine stops,
// and not once it has been tried the most that it may. When it may not, it is dropped.
func resendable(b sim.Bouncer, m sim.Message, r sim.Reason) bool {
	if b.Stopping() {
		b.Drop()
		return false
	}
	if m.Attempts() >= b.MaxAttempts() {
		logging.Warn(
			"message dropped: too many attempts",
			"from", m.Head().From, "dest", m.Dest(), "reason", r, "attempts", m.Attempts(),
			"max_attempts", b.MaxAttempts(),
		)
		b.Drop()
		return false
	}
	return true
}

// Register registers the kinds of OnFail with the codecs, so that a message that holds one can be
// read back.
func Register(c *sim.Codecs) error {
	return sim.Errors(
		c.RegisterOnFail(Retry{}, decodeRetry),
		c.RegisterOnFail(Redirect{}, decodeRedirect),
		c.RegisterOnFail(Compensate{}, decodeCompensate),
		c.RegisterOnFail(Notify{}, decodeNotify),
		c.RegisterOnFail(Escalate{}, decodeEscalate),
		c.RegisterOnFail(Drop{}, decodeDrop),
	)
}
