package engine

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/channels"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/logging"
	"github.com/huykingsofm/project-b/shared/stablemap"
)

// peer is what every engine has to talk to the other engines: its address and channels, the
// router it sends through, what it reads of the others, the bouncer that runs what a message says
// a failed delivery means, and the numbers and acks that keep a message from being lost or taken
// twice. What it takes and what comes back is written to the event log of the engine, which the
// engine gives it with each call that writes to it.
type peer struct {
	data peerData // what a checkpoint holds of the peer

	me sim.StateID

	router MessageRouter
	bounce bouncer
	in     IncomingChannel // what the engine itself reads; the router holds the other end
	view   *StableView     // what the engine reads of the other engines, kept for the tick

	// takenMessages are the messages this engine took in ticks that are not safe yet, in order:
	// when a tick is safe the number of each message it took is acked to its sender.
	takenMessages []takenMessage
	// ackBoard is where the receivers say what they hold safe, and where this engine says it of
	// what it took; nil for an engine that holds nothing for an ack (a replay).
	ackBoard *AckBoard
}

// newPeer makes the peer of the engine with the ID, registers its channel with the router, and
// reads the other engines through the view.
func newPeer(
	me sim.StateID, router MessageRouter, view *StableView, queue int, acks *AckBoard,
) peer {
	in, outgoing := NewChannel(me, queue)
	router.Register(outgoing)
	return peer{
		me: me, router: router, in: in, view: view, ackBoard: acks,
	}
}

// sendToUnsettled keeps a message that is to be sent again with the unsettled messages, to be sent
// at the next flush. The bouncer gives the messages it is asked to resend to it.
func (c *peer) sendToUnsettled(m sim.Message) { c.data.unsettledMessages.AddUnsent(m) }

// config is the configuration that this tick reads, from the copy of the configuration engine in
// the stable view: the first read of the tick is the one the event log records.
func (c *peer) config() config.ReadOnlySnapshot {
	if st, err := c.view.Read(sim.ConfigStateID); err == nil {
		if cs, ok := st.(*sim.ConfigState); ok {
			return cs.Snapshot()
		}
	}
	return config.Defaults()
}

// handleReturnedMessages takes what came back undelivered since the last tick and runs what each
// says to do. A message that came back is settled: it is no longer one that waits for an ack, so it
// is not sent again (which would run what it says to do a second time).
func (c *peer) handleReturnedMessages(tick uint64, rpl eventlog.Log) {
	cfg := c.config()
	for _, r := range channels.Drain(c.in.Back) {
		header := r.Message.Head()
		rpl.Append(c.me, tick, eventlog.Returned{Header: header, Reason: r.Reason})
		c.data.unsettledMessages.ForgetSent(header.Seq)
		c.bounce.handle(cfg, r.Message, r.Reason)
	}
}

// receivedMessages takes the messages that other engines sent since the last tick, in the order
// they arrived, as copies that share nothing with what their senders keep, and writes each to the
// event log.
func (c *peer) receivedMessages(tick uint64, rpl eventlog.Log) []sim.Message {
	var messages []sim.Message
	for _, m := range channels.Drain(c.in.In) {
		if c.seenBefore(m.Head()) {
			continue
		}
		// the sender keeps the message until it is acked, and the actions of the message may change
		// as they run: the engine runs a copy that shares nothing with it
		m = m.Clone()
		rpl.Append(c.me, tick, eventlog.Inbound{Message: m})
		if h := m.Head(); h.Seq != 0 {
			c.takenMessages = append(
				c.takenMessages, takenMessage{tick: tick, sender: h.From, seq: h.Seq},
			)
		}
		messages = append(messages, m)
	}
	return messages
}

// seenBefore reports whether a message with this header, or a later one of the same sender, was
// already taken, and keeps the header. A sender numbers its messages one after another and the
// queue of an engine keeps their order, so a number that is not larger than the last taken is a
// second copy of a message (a message that was sent again after a restart), and it is dropped. A
// message that was not numbered (Seq 0) is never a copy.
func (c *peer) seenBefore(h sim.Header) bool {
	if h.Seq == 0 {
		return false
	}
	if last, _ := c.data.lastRecvSeq.Get(h.From); h.Seq <= last {
		// a sender sends a message again until it is acked, so a copy is what to expect
		logging.Debug(
			"message dropped: it was already taken", "from", h.From, "seq", h.Seq, "engine", c.me,
		)
		return true
	}
	c.data.lastRecvSeq.Set(h.From, h.Seq)
	return false
}

// receivedActions returns the actions of the messages that were sent to an engine whose state is of
// type S, in the order they arrived.
func (c *peer) receivedActions[S sim.State](tick uint64, rpl eventlog.Log) []sim.Action[S] {
	var out []sim.Action[S]
	for _, m := range c.receivedMessages(tick, rpl) {
		if typed, ok := m.(*sim.ActionMessage[S]); ok {
			out = append(out, typed.Actions...)
		}
	}
	return out
}

// flushUnsettledMessages sends what the engine wants to send: first the messages that earlier
// failures made necessary, then what the actions of this tick put in the outbox; and it sends
// again, with the number they have, the messages that went and have not been acked for a while. A
// message that cannot be delivered is handled at once, and the event log records that it came
// back, at the tick of the send. The tick lets go of the acked messages before it calls this.
func (c *peer) flushUnsettledMessages(tick uint64, fresh []sim.Message, rpl eventlog.Log) {
	cfg := c.config()
	for _, m := range append(c.data.unsettledMessages.TakeUnsent(), fresh...) {
		c.data.sentSeq++
		m = m.Stamp(sim.Header{From: c.me, Seq: c.data.sentSeq, Tick: tick})
		reason, ok := c.router.Deliver(m)
		if !ok {
			rpl.Append(c.me, tick, eventlog.Returned{Header: m.Head(), Reason: reason})
			c.bounce.handle(cfg, m, reason)
			continue
		}
		// a message that is not acked is not held: nothing would ever let it go
		if c.ackBoard != nil {
			c.data.unsettledMessages.MarkSent(m, tick, cfg)
		}
	}
	c.resendUnackedMessages(tick)
}

// releaseAckedMessages lets go of the messages that their receivers say they hold safe.
func (c *peer) releaseAckedMessages() {
	if c.ackBoard == nil {
		return
	}
	for _, receiver := range c.data.unsettledMessages.Receivers() {
		c.data.unsettledMessages.Ack(receiver, c.ackBoard.Acked(receiver, c.me))
	}
}

// resendUnackedMessages sends the messages that went and whose ack is late. One whose receiver is
// gone comes back to this engine as a message that could not be delivered, and is let go of: the
// same way as when the receiver is retired while the message waits in its queue.
func (c *peer) resendUnackedMessages(tick uint64) {
	cfg := c.config()
	var gone []int
	for _, i := range c.data.unsettledMessages.Due(tick) {
		m := c.data.unsettledMessages.Message(i)
		if reason, ok := c.router.Deliver(m); !ok && reason == sim.DeliveryFailureRetired {
			c.router.Return(m, reason)
			gone = append(gone, i)
			continue
		}
		// delivered, or full or not started: it waits again
		c.data.unsettledMessages.Reschedule(i, tick, cfg)
	}
	c.data.unsettledMessages.Remove(gone)
}

// publishAcks says to each sender the number up to which this engine holds its messages safe: the
// ticks up to what the event log says is safe are no longer lost to a crash. A log that records
// nothing says every tick is. The engine calls it at the end of each tick.
func (c *peer) publishAcks(rpl eventlog.Log) {
	if c.ackBoard == nil || len(c.takenMessages) == 0 {
		return
	}
	safe := rpl.Safe(c.me)
	n := 0
	for n < len(c.takenMessages) && c.takenMessages[n].tick <= safe {
		m := c.takenMessages[n]
		c.ackBoard.Raise(c.me, m.sender, m.seq)
		n++
	}
	c.takenMessages = append(c.takenMessages[:0], c.takenMessages[n:]...)
}

// retire makes the engine unreachable and gives back what is waiting in its channels.
func (c *peer) retire() {
	c.router.Retire(c.me)
	c.bounce.Stop()
	for _, m := range channels.Drain(c.in.In) {
		c.router.Return(m, sim.DeliveryFailureRetired)
	}
}

// restoreFrom gives the peer what a checkpoint says. It cannot fail: all of it was read and checked
// before the engine was made.
func (c *peer) restoreFrom(data peerData) {
	c.data = data
	if c.ackBoard == nil {
		return
	}
	// what the checkpoint says it took is in a place that no crash takes back: the state was
	// restored from it
	for sender, seq := range c.data.lastRecvSeq.All() {
		c.ackBoard.Raise(c.me, sender, seq)
	}
}

// peerData is what a peer knows of the messages of its engine, and what a checkpoint writes of the
// peer: the rest of a peer is where it is, which a checkpoint does not carry.
type peerData struct {
	sentSeq uint64 // the number of the last message the engine sent
	// lastRecvSeq is the last Seq taken from each sender, in the order the senders were first
	// taken from
	lastRecvSeq stablemap.Map[sim.StateID, uint64]
	// unsettledMessages is what the engine has sent or is to send, until it is acked as safe
	unsettledMessages unsettledMessages
}

// Clone returns a copy that shares nothing with what the peer knows: each unsettled message is
// copied with its Clone.
func (d *peerData) Clone() peerData {
	return peerData{
		sentSeq:           d.sentSeq,
		lastRecvSeq:       d.lastRecvSeq.Clone(),
		unsettledMessages: d.unsettledMessages.Clone(),
	}
}

// Encode writes what the peer knows: the number of the last message of its engine, the last that
// it took from each sender in the order the peer keeps them, and its unsettled messages: the
// messages that are to go and the ones that went and were not acked.
func (d *peerData) Encode(w *binary.Writer) error {
	w.Uvarint(d.sentSeq)
	w.Uvarint(uint64(d.lastRecvSeq.Len()))
	for sender, seq := range d.lastRecvSeq.All() {
		w.Uvarint(uint64(sender))
		w.Uvarint(seq)
	}
	return d.unsettledMessages.Encode(w)
}

// Hash adds what the peer knows to a hash, as the bytes that a checkpoint
// writes for it, so that the hash covers exactly what the checkpoint carries, but for the messages
// that went and were not acked: when a message is acked depends on when the log of its receiver
// is synced, which a replay does not repeat. It returns the error of a peer that cannot be written,
// after it adds a mark of that to the hash, so the hash still tells it from another.
func (d *peerData) Hash(h *sim.Hasher) error {
	held := *d
	held.unsettledMessages = held.unsettledMessages.withoutSent()
	var w binary.Writer
	if err := held.Encode(&w); err != nil {
		h.String("a peer that cannot be written")
		return err
	}
	h.Bytes(w.B)
	return nil
}

// decodePeerData reads what Encode wrote.
func decodePeerData(
	c *sim.Codecs, r *binary.Reader, version codec.Version,
) (peerData, error) {
	d := peerData{sentSeq: r.Uvarint()}
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return peerData{}, err
	}
	if n > uint64(r.Left()) { // a sender takes at least one byte
		return peerData{}, fmt.Errorf("%w: %d senders", binary.ErrShort, n)
	}
	for range n {
		sender := r.Uint[sim.StateID]()
		if d.lastRecvSeq.Has(sender) {
			return peerData{}, fmt.Errorf("%w: the sender %d is twice", binary.ErrBadValue, sender)
		}
		d.lastRecvSeq.Set(sender, r.Uvarint())
	}
	var err error
	if d.unsettledMessages, err = decodeUnsettledMessages(c, r, version); err != nil {
		return peerData{}, err
	}
	return d, r.Err()
}

// takenMessage is a message that an engine took, with the tick at which it took it.
type takenMessage struct {
	tick   uint64
	sender sim.StateID
	seq    uint64
}
