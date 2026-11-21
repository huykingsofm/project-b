package engine

import (
	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
)

// bouncer decides what a message that could not be delivered means. The message says it itself
// (OnFail), as a decision of one of the kinds in action.DecisionKind, and the bouncer runs the
// decision from a table by kind. What it makes is kept to be sent at the next flush.
type bouncer struct {
	pending  []action.Message
	notice   func(actor any, key string, args []any)
	stopping bool
	dropped  uint64
}

// decide is the table of what each kind of decision does. A kind that is not in it is dropped.
var decide = map[action.DecisionKind]func(b *bouncer, l action.Message, d action.Decision){
	action.DecisionRetry: func(b *bouncer, l action.Message, _ action.Decision) {
		b.pending = append(b.pending, l.Again())
	},
	action.DecisionRedirect: func(b *bouncer, l action.Message, d action.Decision) {
		b.pending = append(b.pending, l.Redirected(d.To).Again())
	},
	action.DecisionCompensate: func(b *bouncer, _ action.Message, d action.Decision) {
		b.pending = append(b.pending, d.With)
	},
	action.DecisionNotify: func(b *bouncer, l action.Message, d action.Decision) {
		if b.notice != nil && l.Subject() != nil {
			b.notice(l.Subject(), d.Key, d.Args)
		}
	},
	action.DecisionEscalate: func(b *bouncer, l action.Message, d action.Decision) {
		b.pending = append(b.pending, l.Redirected(d.To))
	},
	action.DecisionDrop: func(b *bouncer, _ action.Message, _ action.Decision) { b.dropped++ },
}

// handle runs what the message says to do about the failure. A message that has been tried
// bounce.max_attempts times is dropped, and while the engine stops nothing is sent again.
func (b *bouncer) handle(cfg *config.Snapshot, l action.Message, r action.Reason) {
	d := l.Bounce(r)
	switch d.Kind {
	case action.DecisionRetry, action.DecisionRedirect:
		if b.stopping || l.Attempts() >= action.MaxAttempts.Get(cfg) {
			d = action.Decision{Kind: action.DecisionDrop}
		}
	case action.DecisionEscalate:
		if b.stopping {
			d = action.Decision{Kind: action.DecisionDrop}
		}
	}
	run, ok := decide[d.Kind]
	if !ok {
		run = decide[action.DecisionDrop]
	}
	run(b, l, d)
}

// take returns the messages that are to be sent again, and forgets them.
func (b *bouncer) take() []action.Message {
	out := b.pending
	b.pending = nil
	return out
}
