package engine

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"math/rand/v2"

	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/shared/channels"
	"github.com/huykingsofm/project-b/server/internal/shared/logging"
)

// replayLogHashEvery is every how many ticks an engine records the hash of its state: it costs as
// much as the state is big, so it is taken at checkpoints and not at every tick. A replay that
// finds a difference at a checkpoint runs the ticks since the one before it again to find where it
// began. Each engine has its own checkpoints (see recordHash), so that the engines do not all hash
// at the same tick.
var replayLogHashEvery = config.NewVar("replay_log.hash_every_ticks", 20, config.Range(1, 100000))

// cell is what every engine has to talk to the other engines: its address and channels, the
// router it sends through, the configuration it reads, the replay log it writes, and the
// bouncer that runs what a message says a failed delivery means.
type cell struct {
	me             message.Address
	router         *Router
	in             IncomingChannel // what the engine itself reads; the router holds the other end
	cfg            *config.Store
	cfgVersionSeen uint64
	replayLog      replay.Log
	view           *StableStateView // what the engine reads of the other engines, kept for the tick
	seq            uint64
	lastSeq        map[message.Address]uint64 // the last Seq taken from each sender
	bounce         bouncer
}

func newCell(
	me message.Address,
	router *Router,
	cfg *config.Store,
	rpl replay.Log,
	stable *StableStateRegistry,
	queue int,
) cell {
	in, out := NewChannel(me, queue)
	router.Register(out)
	return cell{
		me: me, router: router, in: in, cfg: cfg, replayLog: rpl, view: stable.View(me, rpl),
		lastSeq: map[message.Address]uint64{},
	}
}

// newRand makes the random numbers of an engine: the seed of the cluster and the address of the
// engine decide them, so the same inputs give the same run.
func newRand(seed uint64, me message.Address) *rand.Rand {
	return rand.New(rand.NewPCG(seed, uint64(me.Scope)<<32|uint64(me.ID)))
}

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

// recordHash writes the hash of the state to the replay log at every replayLogHashEvery-th tick.
// Where the count starts depends on the address of the engine (its ID times 3, plus its scope:
// there are three scopes, and the IDs of one scope follow each other), so that the engines of a
// cluster do not all hash in the same tick and make a peak of work there. hash is only called when
// the log wants a hash.
func (c *cell) recordHash(cfg *config.Snapshot, tick uint64, hash func() uint64) {
	every := uint64(replayLogHashEvery.Get(cfg))
	offset := uint64(c.me.ID)*3 + uint64(c.me.Scope)
	if (tick+offset)%every == 0 && c.replayLog.Wants(replay.KindHash) {
		c.replayLog.Append(c.me, tick, replay.Hash{Value: hash()})
	}
}

// endTick tells the replay log that the engine has recorded all it will for the tick.
func (c *cell) endTick(tick uint64) { c.replayLog.EndTick(c.me, tick) }

// settle takes what came back undelivered since the last tick and runs what each says to do.
func (c *cell) settle(cfg *config.Snapshot, tick uint64) {
	for _, r := range channels.Drain(c.in.Back) {
		c.replayLog.Append(
			c.me, tick, replay.Returned{Header: r.Message.Head(), Reason: r.Reason},
		)
		c.bounce.handle(cfg, r.Message, r.Reason)
	}
}

// received takes the messages that other engines sent since the last tick, in the order they
// arrived, and writes each to the replay log.
func (c *cell) received(tick uint64) []message.Message {
	var messages []message.Message
	for _, m := range channels.Drain(c.in.In) {
		if c.seenBefore(m.Head()) {
			continue
		}
		c.replayLog.Append(c.me, tick, replay.Inbound{Message: m})
		messages = append(messages, m)
	}
	return messages
}

// seenBefore reports whether a message with this header, or a later one of the same sender, was
// already taken, and keeps the header. A sender numbers its messages one after another and the
// queue of an engine keeps their order, so a number that is not larger than the last taken is a
// second copy of a message (a message that was sent again after a restart), and it is dropped. A
// message that was not numbered (Seq 0) is never a copy.
func (c *cell) seenBefore(h message.Header) bool {
	if h.Seq == 0 {
		return false
	}
	if h.Seq <= c.lastSeq[h.From] {
		logging.Warn(
			"message dropped: it was already taken", "from", h.From, "seq", h.Seq, "engine", c.me,
		)
		return true
	}
	c.lastSeq[h.From] = h.Seq
	return false
}

// receivedActions returns the actions of the messages that were sent to an engine of the scope of
// A, in the order they arrived.
func (c *cell) receivedActions[A action.Action](tick uint64) []A {
	var out []A
	for _, m := range c.received(tick) {
		if scoped, ok := m.(*action.ScopedMessage[A]); ok {
			out = append(out, scoped.Actions...)
		}
	}
	return out
}

// flush sends what the engine wants to send: first the messages that earlier failures made
// necessary, then what the actions of this tick put in the outbox. A message that cannot be
// delivered is handled at once, and the replay log records that it came back, at the tick of
// the send.
func (c *cell) flush(cfg *config.Snapshot, tick uint64, outbox []message.Message) {
	for _, m := range append(c.bounce.take(), outbox...) {
		c.seq++
		m = m.Stamp(message.Header{From: c.me, Seq: c.seq})
		if reason, ok := c.router.Deliver(m); !ok {
			c.replayLog.Append(c.me, tick, replay.Returned{Header: m.Head(), Reason: reason})
			c.bounce.handle(cfg, m, reason)
		}
	}
}

// retire makes the engine unreachable and gives back what is waiting in its channels.
func (c *cell) retire() {
	c.router.Retire(c.me)
	c.bounce.stopping = true
	for _, m := range channels.Drain(c.in.In) {
		c.router.Return(m, message.Retired)
	}
}
