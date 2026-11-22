package engine

import (
	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/shared/logging"
)

// bouncer is what a message that could not be delivered asks of the engine that sent it: the
// message says itself what a failed delivery means (its OnFail), and runs on the bouncer through
// the interface action.Bouncer. What it makes is kept to be sent at the next flush.
type bouncer struct {
	pending  []action.Message
	notice   func(actor any, key string, args []any)
	stopping bool
	dropped  uint64
	cfg      *config.Snapshot // of the tick: how many attempts a message may have
}

var _ action.Bouncer = (*bouncer)(nil)

// handle runs what the message says to do about the failure. A message with no OnFail is dropped
// and logged.
func (b *bouncer) handle(cfg *config.Snapshot, m action.Message, r action.Reason) {
	b.cfg = cfg
	onFail := m.OnFail()
	if onFail == nil {
		logging.Error(
			"message dropped: it says nothing of what a failed delivery means",
			"from", m.Head().From, "dest", m.Dest(), "reason", r,
		)
		b.Drop()
		return
	}
	onFail.Handle(b, m, r)
}

// Resend keeps a message to be sent at the next flush.
func (b *bouncer) Resend(m action.Message) { b.pending = append(b.pending, m) }

// Tell tells an actor something. An actor that is nil, or an engine that has no one to tell, tells
// nothing.
func (b *bouncer) Tell(actor any, key string, args []string) {
	if b.notice == nil || actor == nil {
		return
	}
	out := make([]any, len(args))
	for i, a := range args {
		out[i] = a
	}
	b.notice(actor, key, out)
}

// Drop forgets a message and counts it.
func (b *bouncer) Drop() { b.dropped++ }

// Stopping reports whether the engine is stopping.
func (b *bouncer) Stopping() bool { return b.stopping }

// MaxAttempts is how many times a message is tried.
func (b *bouncer) MaxAttempts() int { return action.MaxAttempts.Get(b.cfg) }

// take returns the messages that are to be sent again, and forgets them.
func (b *bouncer) take() []action.Message {
	out := b.pending
	b.pending = nil
	return out
}
