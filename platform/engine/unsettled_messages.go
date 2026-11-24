package engine

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// How long a message that was sent waits for its ack before it is sent again: the first time, then
// longer by the step each time, up to the most, all in ticks of the sender. The receiver drops a
// message it has taken, so a second copy costs a channel slot and nothing else; the numbers are
// those of an ack that comes within a sync of the log of the receiver, which is a second or so.
var (
	unsettledResendFirst = config.NewVar(
		"unsettled_messages.resend_first_ticks", 20, config.Range(1, 1000000),
	)
	unsettledResendStep = config.NewVar(
		"unsettled_messages.resend_step_ticks", 20, config.Range(1, 1000000),
	)
	unsettledResendMost = config.NewVar(
		"unsettled_messages.resend_most_ticks", 200, config.Range(1, 1000000),
	)
)

// sentMessage is a message that went into the channel of its receiver and has not been acked yet.
type sentMessage struct {
	message sim.Message
	due     uint64 // the tick at which it is sent again
	wait    uint64 // how long it waited for the last time
}

// UnsettledMessages holds what an engine has sent or is to send, for as long as it can be lost: the
// messages that are to go (a message that its OnFail sends again, which gets a new number when it
// goes) and the messages that went and have not been acked as safe (each is sent again, with the
// number it has, until the receiver has it in a place that no crash takes back). Both are in a
// checkpoint. Only the goroutine of the engine uses it.
type UnsettledMessages struct {
	unsent []sim.Message
	sent   []sentMessage // in the order they were sent, which is the order of their numbers
}

// AddUnsent puts a message that is to go at the next flush.
func (u *UnsettledMessages) AddUnsent(m sim.Message) { u.unsent = append(u.unsent, m) }

// Unsent is the messages that are to go, in the order they will. The list is a view.
func (u *UnsettledMessages) Unsent() []sim.Message { return u.unsent }

// SetUnsent sets the messages that are to go, as a checkpoint says.
func (u *UnsettledMessages) SetUnsent(messages []sim.Message) { u.unsent = messages }

// TakeUnsent returns the messages that are to go and forgets them.
func (u *UnsettledMessages) TakeUnsent() []sim.Message {
	out := u.unsent
	u.unsent = nil
	return out
}

// Sent is the messages that went and have not been acked, in the order they went.
func (u *UnsettledMessages) Sent() []sim.Message {
	out := make([]sim.Message, len(u.sent))
	for i, s := range u.sent {
		out[i] = s.message
	}
	return out
}

// SetSent sets the messages that went and have not been acked, as a checkpoint says: each is sent
// again at the next flush.
func (u *UnsettledMessages) SetSent(messages []sim.Message) {
	u.sent = make([]sentMessage, len(messages))
	for i, m := range messages {
		u.sent[i] = sentMessage{message: m}
	}
}

// Len is how many messages it holds.
func (u *UnsettledMessages) Len() int { return len(u.unsent) + len(u.sent) }

// MarkSent says that a message went into the channel of its receiver at a tick.
func (u *UnsettledMessages) MarkSent(m sim.Message, tick uint64, cfg *config.Snapshot) {
	wait := uint64(unsettledResendFirst.Get(cfg))
	u.sent = append(u.sent, sentMessage{message: m, due: tick + wait, wait: wait})
}

// Due returns the indexes of the messages that are to be sent again at the tick, in order.
func (u *UnsettledMessages) Due(tick uint64) []int {
	var due []int
	for i, s := range u.sent {
		if s.due <= tick {
			due = append(due, i)
		}
	}
	return due
}

// Message is the message that went, by its index.
func (u *UnsettledMessages) Message(i int) sim.Message { return u.sent[i].message }

// Reschedule says that the message at the index was sent again at the tick: the next time it waits
// the step longer, up to the most.
func (u *UnsettledMessages) Reschedule(i int, tick uint64, cfg *config.Snapshot) {
	s := &u.sent[i]
	s.wait = min(s.wait+uint64(unsettledResendStep.Get(cfg)), uint64(unsettledResendMost.Get(cfg)))
	s.due = tick + s.wait
}

// Remove forgets the messages at the indexes, which Due returned, in order.
func (u *UnsettledMessages) Remove(indexes []int) {
	if len(indexes) == 0 {
		return
	}
	kept := u.sent[:0]
	next := 0
	for i, s := range u.sent {
		if next < len(indexes) && indexes[next] == i {
			next++
			continue
		}
		kept = append(kept, s)
	}
	clear(u.sent[len(kept):])
	u.sent = kept
}

// Ack forgets the messages for a receiver that have a number up to the one given: the receiver has
// them in a place that no crash takes back.
func (u *UnsettledMessages) Ack(receiver sim.StateID, upTo uint64) {
	kept := u.sent[:0]
	for _, s := range u.sent {
		if s.message.Dest() == receiver && s.message.Head().Seq <= upTo {
			continue
		}
		kept = append(kept, s)
	}
	clear(u.sent[len(kept):])
	u.sent = kept
}

// Receivers is the receivers that have a message that was not acked, each once, in order.
func (u *UnsettledMessages) Receivers() []sim.StateID {
	var out []sim.StateID
	seen := map[sim.StateID]bool{}
	for _, s := range u.sent {
		if d := s.message.Dest(); !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

// Encode writes the messages that are to go, and then the ones that went and were not acked, each
// whole. What it waits for each is not written: after a restore every one is sent again.
func (u *UnsettledMessages) Encode(w *binary.Writer, withSent bool) error {
	w.Uvarint(uint64(len(u.unsent)))
	for _, m := range u.unsent {
		if err := sim.EncodeMessage(w, m); err != nil {
			return fmt.Errorf("a message that is to go: %w", err)
		}
	}
	if !withSent {
		return nil
	}
	w.Uvarint(uint64(len(u.sent)))
	for _, s := range u.sent {
		if err := sim.EncodeMessage(w, s.message); err != nil {
			return fmt.Errorf("a message that went and was not acked: %w", err)
		}
	}
	return nil
}

// decodeUnsettledMessages reads what Encode wrote with the sent messages.
func decodeUnsettledMessages(
	c *sim.Codecs, r *binary.Reader, version codec.Version,
) (UnsettledMessages, error) {
	var u UnsettledMessages
	for _, what := range []string{"that is to go", "that went and was not acked"} {
		n := r.Uvarint()
		if err := r.Err(); err != nil {
			return UnsettledMessages{}, err
		}
		if n > uint64(r.Left()) { // a message takes at least one byte
			return UnsettledMessages{}, fmt.Errorf("%w: %d messages", binary.ErrShort, n)
		}
		messages := make([]sim.Message, 0, n)
		for range n {
			m, err := c.DecodeMessage(r, version)
			if err != nil {
				return UnsettledMessages{}, fmt.Errorf("a message %s: %w", what, err)
			}
			messages = append(messages, m)
		}
		if what == "that is to go" {
			u.SetUnsent(messages)
		} else {
			u.SetSent(messages)
		}
	}
	return u, r.Err()
}

// AckBoard is where a receiver says, for each sender, up to which of its numbers it has taken
// messages and holds them in a place that no crash takes back, and where a sender reads it. An ack
// is not a message: it is runtime only, it is never written to a log or a checkpoint, and it only
// goes up, so one that is late or never read does no harm.
type AckBoard struct {
	mu    sync.Mutex
	slots map[ackKey]*atomic.Uint64
}

type ackKey struct{ receiver, sender sim.StateID }

// NewAckBoard makes an empty board.
func NewAckBoard() *AckBoard { return &AckBoard{slots: map[ackKey]*atomic.Uint64{}} }

// Slot is where the receiver says the number for the sender. A receiver and a sender keep it.
func (b *AckBoard) Slot(receiver, sender sim.StateID) *atomic.Uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := ackKey{receiver, sender}
	slot, ok := b.slots[key]
	if !ok {
		slot = &atomic.Uint64{}
		b.slots[key] = slot
	}
	return slot
}

// raise sets a slot to the number given, when it is more than it holds.
func raise(slot *atomic.Uint64, seq uint64) {
	for {
		old := slot.Load()
		if old >= seq || slot.CompareAndSwap(old, seq) {
			return
		}
	}
}
