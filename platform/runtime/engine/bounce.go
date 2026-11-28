package engine

import (
	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/logging"
)

// bouncerAt is the bouncer as the kinds of OnFail see it in a tick: with the configuration of the
// tick, which says how many times a message is tried.
type bouncerAt struct {
	*bouncer
	cfg config.ReadOnlySnapshot
}

// MaxAttempts is how many times a message is tried.
func (b bouncerAt) MaxAttempts() int { return sim.MaxAttempts.Get(b.cfg) }

var _ sim.Bouncer = bouncerAt{}

// bouncer is what a message that could not be delivered asks of the engine that sent it: the
// message says itself what a failed delivery means (its OnFail), and runs on the bouncer through
// the interface sim.Bouncer. A message that it is to send again is given to resend, which the
// engine sets to keep it with its unsettled messages, to be sent at the next flush.
type bouncer struct {
	resend   func(m sim.Message)
	stopping bool
	dropped  uint64
}

// Resend keeps a message to be sent at the next flush.
func (b *bouncer) Resend(m sim.Message) { b.resend(m) }

// Drop forgets a message and counts it.
func (b *bouncer) Drop() { b.dropped++ }

// Stopping reports whether the engine is stopping.
func (b *bouncer) Stopping() bool { return b.stopping }

// Stop makes the engine stopping: from now on nothing is sent again.
func (b *bouncer) Stop() { b.stopping = true }

// handle runs what the message says to do about the failure. A message with no OnFail is dropped
// and logged.
func (b *bouncer) handle(cfg config.ReadOnlySnapshot, m sim.Message, r sim.DeliveryFailure) {
	onFail := m.OnFail()
	if onFail == nil {
		logging.Error(
			"message dropped: it says nothing of what a failed delivery means",
			"from", m.Head().From, "dest", m.Dest(), "reason", r,
		)
		b.Drop()
		return
	}
	onFail.Handle(bouncerAt{b, cfg}, m, r)
}
