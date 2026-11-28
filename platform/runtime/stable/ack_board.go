package stable

import (
	"sync"
	"sync/atomic"

	"github.com/huykingsofm/project-b/platform/sim"
)

// AckBoard is where a receiver says, for each sender, up to which of its numbers it has taken
// messages and holds them in a place that no crash takes back, and where a sender reads it. An ack
// is not a message: it is runtime only, it is never written to a log or a checkpoint, and it only
// goes up, so one that is late or never read does no harm.
type AckBoard struct {
	slots sync.Map // ackKey to *atomic.Uint64: a pair is added once and never removed
}

// NewAckBoard makes an empty board.
func NewAckBoard() *AckBoard { return &AckBoard{} }

// Raise says that the receiver holds the messages of the sender, up to the number seq. It never
// goes down: a number that is not more than the one the board holds is left out.
func (b *AckBoard) Raise(receiver, sender sim.StateID, seq uint64) {
	key := ackKey{receiver, sender}
	v, ok := b.slots.Load(key)
	if !ok {
		v, _ = b.slots.LoadOrStore(key, &atomic.Uint64{})
	}
	slot := v.(*atomic.Uint64)
	for {
		old := slot.Load()
		if old >= seq || slot.CompareAndSwap(old, seq) {
			return
		}
	}
}

// Acked is the number up to which the receiver says it holds the messages of the sender, or 0 when
// it has said nothing.
func (b *AckBoard) Acked(receiver, sender sim.StateID) uint64 {
	if v, ok := b.slots.Load(ackKey{receiver, sender}); ok {
		return v.(*atomic.Uint64).Load()
	}
	return 0
}

type ackKey struct{ receiver, sender sim.StateID }
