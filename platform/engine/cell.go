package engine

import (
	"fmt"
	"slices"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/channels"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/logging"
	"github.com/huykingsofm/project-b/shared/rand"
)

// replayLogHashEvery is every how many ticks an engine records the hash of its state: it costs as
// much as the state is big, so it is taken at checkpoints and not at every tick. A replay that
// finds a difference at a checkpoint runs the ticks since the one before it again to find where it
// began. Each engine has its own checkpoints (see recordHash), so that the engines do not all hash
// at the same tick.
var replayLogHashEvery = config.NewVar("replay_log.hash_every_ticks", 20, config.Range(1, 100000))

// ConfigSource is what an engine needs of the configuration: the snapshot it takes once for each
// tick. A config.Store is one; a replay gives the engines the version that the log says they read.
type ConfigSource interface {
	Snapshot() *config.Snapshot
}

// cell is what every engine has to talk to the other engines: its address and channels, the
// router it sends through, the configuration it reads, the replay log it writes, and the
// bouncer that runs what a message says a failed delivery means.
type cell struct {
	me sim.StateID

	router MessageRouter
	bounce bouncer
	in     IncomingChannel // what the engine itself reads; the router holds the other end

	cfg            ConfigSource
	cfgVersionSeen uint64

	replayLog replay.Log

	view *StableView // what the engine reads of the other engines, kept for the tick

	sentSeq     uint64
	lastRecvSeq map[sim.StateID]uint64 // the last Seq taken from each sender
	// unsettledMessages is what the engine has sent or is to send, until it is acked as safe
	unsettledMessages *unsettledMessages
	// takenMessages are the messages this engine took in ticks that are not safe yet, in order:
	// when a tick is safe the number of each message it took is acked to its sender.
	takenMessages []takenMessage
	// ackBoard is where the receivers say what they hold safe, and where this engine says it of
	// what it took; nil for an engine that holds nothing for an ack (a replay).
	ackBoard *AckBoard

	rng *rand.Rand // the random numbers of the engine and where they are
}

// takenMessage is a message that an engine took, with the tick at which it took it.
type takenMessage struct {
	tick   uint64
	sender sim.StateID
	seq    uint64
}

func newCell(
	me sim.StateID,
	router MessageRouter,
	cfg ConfigSource,
	rpl replay.Log,
	stable StableStore,
	queue int,
	seed uint64,
	acks *AckBoard,
) cell {
	in, outgoing := NewChannel(me, queue)
	router.Register(outgoing)
	// The seed of the cluster and the ID of the engine decide the random numbers, so the same
	// inputs give the same run.
	return cell{
		me: me, router: router, in: in, cfg: cfg, replayLog: rpl, view: stable.View(me, rpl),
		lastRecvSeq: map[sim.StateID]uint64{}, rng: rand.NewPCG(seed, uint64(me)),
		unsettledMessages: &unsettledMessages{},
		ackBoard:          acks,
	}
}

// sendToUnsettled keeps a message that is to be sent again with the unsettled messages, to be sent
// at the next flush. The bouncer gives the messages it is asked to resend to it.
func (c *cell) sendToUnsettled(m sim.Message) { c.unsettledMessages.AddUnsent(m) }

// StateID is the ID of the state of the engine, which is also its address.
func (c *cell) StateID() sim.StateID { return c.me }

// snapshot takes the configuration once for the whole tick, and records the version when the
// engine reads a new one.
func (c *cell) snapshot(tick uint64) *config.Snapshot {
	s := c.cfg.Snapshot()
	if s.Version() != c.cfgVersionSeen {
		c.cfgVersionSeen = s.Version()
		c.replayLog.Append(c.me, tick, replay.ConfigRead{Version: s.Version()})
	}
	return s
}

// endTick closes the tick for the replay log: it writes the hash of the state, at every
// replayLogHashEvery-th tick, as the last record of the tick, and tells the log that the engine has
// recorded all it will for the tick. Where the count starts depends on the ID of the engine, so
// that the engines of a cluster do not all hash in the same tick and make a peak of work there. A
// log that records nothing is given the hash all the same: it is a place holder, and what it is
// given costs nothing it could save.
func (c *cell) endTick(cfg *config.Snapshot, tick uint64, hash func() uint64) {
	every := uint64(replayLogHashEvery.Get(cfg))
	offset := uint64(c.me)
	if (tick+offset)%every == 0 {
		c.replayLog.Append(c.me, tick, replay.Hash{Value: hash()})
	}
	c.replayLog.EndTick(c.me, tick)
	c.publishAcks()
}

// handleReturnedMessages takes what came back undelivered since the last tick and runs what each
// says to do.
func (c *cell) handleReturnedMessages(cfg *config.Snapshot, tick uint64) {
	for _, r := range channels.Drain(c.in.Back) {
		c.replayLog.Append(
			c.me, tick, replay.Returned{Header: r.Message.Head(), Reason: r.Reason},
		)
		c.bounce.handle(cfg, r.Message, r.Reason)
	}
}

// receivedMessages takes the messages that other engines sent since the last tick, in the order
// they arrived, and writes each to the replay log.
func (c *cell) receivedMessages(tick uint64) []sim.Message {
	var messages []sim.Message
	for _, m := range channels.Drain(c.in.In) {
		if c.seenBefore(m.Head()) {
			continue
		}
		c.replayLog.Append(c.me, tick, replay.Inbound{Message: m})
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
func (c *cell) seenBefore(h sim.Header) bool {
	if h.Seq == 0 {
		return false
	}
	if h.Seq <= c.lastRecvSeq[h.From] {
		// a sender sends a message again until it is acked, so a copy is what to expect
		logging.Debug(
			"message dropped: it was already taken", "from", h.From, "seq", h.Seq, "engine", c.me,
		)
		return true
	}
	c.lastRecvSeq[h.From] = h.Seq
	return false
}

// receivedActions returns the actions of the messages that were sent to an engine whose state is of
// type S, in the order they arrived.
func (c *cell) receivedActions[S sim.State](tick uint64) []sim.Action[S] {
	var out []sim.Action[S]
	for _, m := range c.receivedMessages(tick) {
		if typed, ok := m.(*sim.ActionMessage[S]); ok {
			out = append(out, typed.Actions...)
		}
	}
	return out
}

// flushUnsettledMessages sends what the engine wants to send: first the messages that earlier
// failures made necessary, then what the actions of this tick put in the outbox; and it sends
// again, with the number they have, the messages that went and have not been acked for a while. A
// message that cannot be delivered is handled at once, and the replay log records that it came
// back, at the tick of the send.
func (c *cell) flushUnsettledMessages(cfg *config.Snapshot, tick uint64, fresh []sim.Message) {
	c.releaseAckedMessages()
	for _, m := range append(c.unsettledMessages.TakeUnsent(), fresh...) {
		c.sentSeq++
		m = m.Stamp(sim.Header{From: c.me, Seq: c.sentSeq, Tick: tick})
		reason, ok := c.router.Deliver(m)
		if !ok {
			c.replayLog.Append(c.me, tick, replay.Returned{Header: m.Head(), Reason: reason})
			c.bounce.handle(cfg, m, reason)
			continue
		}
		// a message that is not acked is not held: nothing would ever let it go
		if c.ackBoard != nil {
			c.unsettledMessages.MarkSent(m, tick, cfg)
		}
	}
	c.sendAgain(cfg, tick)
}

// releaseAckedMessages lets go of the messages that their receivers say they hold safe.
func (c *cell) releaseAckedMessages() {
	if c.ackBoard == nil {
		return
	}
	for _, receiver := range c.unsettledMessages.Receivers() {
		c.unsettledMessages.Ack(receiver, c.ackBoard.Acked(receiver, c.me))
	}
}

// sendAgain sends the messages that went and whose ack is late. One whose receiver is gone comes
// back to this engine as a message that could not be delivered, and is let go of: the same way as
// when the receiver is retired while the message waits in its queue.
func (c *cell) sendAgain(cfg *config.Snapshot, tick uint64) {
	var gone []int
	for _, i := range c.unsettledMessages.Due(tick) {
		m := c.unsettledMessages.Message(i)
		if reason, ok := c.router.Deliver(m); !ok && reason == sim.DeliveryFailureRetired {
			c.router.Return(m, reason)
			gone = append(gone, i)
			continue
		}
		// delivered, or full or not started: it waits again
		c.unsettledMessages.Reschedule(i, tick, cfg)
	}
	c.unsettledMessages.Remove(gone)
}

// publishAcks says to each sender the number up to which this engine holds its messages safe: the
// ticks up to what the replay log says is safe are no longer lost to a crash. A log that records
// nothing says every tick is.
func (c *cell) publishAcks() {
	if c.ackBoard == nil || len(c.takenMessages) == 0 {
		return
	}
	safe := c.replayLog.Safe(c.me)
	n := 0
	for n < len(c.takenMessages) && c.takenMessages[n].tick <= safe {
		m := c.takenMessages[n]
		c.ackBoard.Raise(c.me, m.sender, m.seq)
		n++
	}
	c.takenMessages = append(c.takenMessages[:0], c.takenMessages[n:]...)
}

// retire makes the engine unreachable and gives back what is waiting in its channels.
func (c *cell) retire() {
	c.router.Retire(c.me)
	c.bounce.Stop()
	for _, m := range channels.Drain(c.in.In) {
		c.router.Return(m, sim.DeliveryFailureRetired)
	}
}

// engineHash is the hash of an engine: of everything a checkpoint of it holds that the engine goes
// on from: its state, what it waits for (the scheduler), and what only it knows (the place of its
// random numbers, the number of its messages and the last it took from each sender, the version of
// the configuration it read, and the messages it holds to send again). A restore that differs
// from the engine it came from in any of them has another hash, and a replay finds it.
func (c *cell) engineHash[S sim.State](
	stateHash uint64, sched *sim.Scheduler[S],
) uint64 {
	h := sim.NewHasher()
	h.U64(stateHash)
	sched.Hash(&h)
	c.hashInto(&h)
	return h.Sum()
}

// hashInto adds what every engine knows besides its state to a hash, as the bytes that a
// checkpoint writes for it, so that the hash covers exactly what the checkpoint carries.
func (c *cell) hashInto(h *sim.Hasher) {
	var w binary.Writer
	if err := c.encode(&w, false); err != nil {
		h.String("a cell that cannot be written") // the hash still tells it from another
		return
	}
	h.Bytes(w.B)
}

// checkpoint writes the checkpoint of an engine, given its state.
func (c *cell) checkpoint[S sim.State](
	tick uint64, st S, sched *sim.Scheduler[S],
) ([]byte, error) {
	var w binary.Writer
	w.Uvarint(1) // the version of the data
	w.Uvarint(uint64(c.me))
	w.Uvarint(tick)
	if err := st.Encode(&w); err != nil {
		return nil, fmt.Errorf("%v: the state: %w", c.me, err)
	}
	if err := sched.Encode(&w); err != nil {
		return nil, fmt.Errorf("%v: the scheduler: %w", c.me, err)
	}
	if err := c.encode(&w, true); err != nil {
		return nil, fmt.Errorf("%v: %w", c.me, err)
	}
	return w.B, nil
}

// encode writes what every engine knows besides its state: the place of its random numbers, the
// number of its messages and the last that it took from each sender, the version of the
// configuration it read, and its unsettled messages: the messages that are to go and, with
// withSent, the ones that went and were not acked. The hash leaves out the second: when a message
// is acked depends on when the log of its receiver is synced, which a replay does not repeat.
func (c *cell) encode(w *binary.Writer, withSent bool) error {
	if err := c.rng.Encode(w); err != nil {
		return fmt.Errorf("its random numbers: %w", err)
	}
	w.Uvarint(c.sentSeq)
	w.Uvarint(c.cfgVersionSeen)
	senders := make([]sim.StateID, 0, len(c.lastRecvSeq))
	for sender := range c.lastRecvSeq {
		senders = append(senders, sender)
	}
	slices.Sort(senders)
	w.Uvarint(uint64(len(senders)))
	for _, sender := range senders {
		w.Uvarint(uint64(sender))
		w.Uvarint(c.lastRecvSeq[sender])
	}
	return c.unsettledMessages.Encode(w, withSent)
}

// restore gives the cell what a checkpoint says. It cannot fail: all of it was read and checked
// before the engine was made.
func (c *cell) restore(rc restoredCell) {
	c.rng = rc.Rand()
	c.sentSeq, c.cfgVersionSeen = rc.SentSeq(), rc.CfgVersionSeen()
	c.lastRecvSeq = rc.LastRecvSeq()
	held := rc.UnsettledMessages()
	c.unsettledMessages.SetUnsent(held.Unsent())
	c.unsettledMessages.SetSent(held.Sent())
	// what the checkpoint says it took is in a place that no crash takes back: the state was
	// restored from it
	for sender, seq := range c.lastRecvSeq {
		if c.ackBoard != nil {
			c.ackBoard.Raise(c.me, sender, seq)
		}
	}
}
