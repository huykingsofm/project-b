package engine

import (
	"github.com/huykingsofm/project-b/platform/replay"
	"math/rand/v2"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/channels"
	"github.com/huykingsofm/project-b/shared/logging"
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
	me             sim.StateID
	router         MessageRouter
	in             IncomingChannel // what the engine itself reads; the router holds the other end
	cfg            ConfigSource
	cfgVersionSeen uint64
	replayLog      replay.Log
	view           *StableView // what the engine reads of the other engines, kept for the tick
	seq            uint64
	lastSeq        map[sim.StateID]uint64 // the last Seq taken from each sender
	pcg            *rand.PCG              // the random numbers of the engine and where they are
	rng            *rand.Rand
	bounce         bouncer
}

func newCell(
	me sim.StateID,
	router MessageRouter,
	cfg ConfigSource,
	rpl replay.Log,
	stable StableStore,
	queue int,
	seed uint64,
) cell {
	in, out := NewChannel(me, queue)
	router.Register(out)
	// The seed of the cluster and the ID of the engine decide the random numbers, so the same
	// inputs give the same run.
	pcg := rand.NewPCG(seed, uint64(me))
	return cell{
		me: me, router: router, in: in, cfg: cfg, replayLog: rpl, view: stable.View(me, rpl),
		lastSeq: map[sim.StateID]uint64{}, pcg: pcg, rng: rand.New(pcg),
	}
}

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
}

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
func (c *cell) received(tick uint64) []sim.Message {
	var messages []sim.Message
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
func (c *cell) seenBefore(h sim.Header) bool {
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

// receivedActions returns the actions of the messages that were sent to an engine whose state is of
// type S, in the order they arrived.
func (c *cell) receivedActions[S sim.State](tick uint64) []sim.Action[S] {
	var out []sim.Action[S]
	for _, m := range c.received(tick) {
		if typed, ok := m.(*sim.ActionMessage[S]); ok {
			out = append(out, typed.Actions...)
		}
	}
	return out
}

// flush sends what the engine wants to send: first the messages that earlier failures made
// necessary, then what the actions of this tick put in the outbox. A message that cannot be
// delivered is handled at once, and the replay log records that it came back, at the tick of
// the send.
func (c *cell) flush(cfg *config.Snapshot, tick uint64, outbox []sim.Message) {
	for _, m := range append(c.bounce.take(), outbox...) {
		c.seq++
		m = m.Stamp(sim.Header{From: c.me, Seq: c.seq, Tick: tick})
		if reason, ok := c.router.Deliver(m); !ok {
			c.replayLog.Append(c.me, tick, replay.Returned{Header: m.Head(), Reason: reason})
			c.bounce.handle(cfg, m, reason)
		}
	}
}

// retire makes the engine unreachable and gives back what is waiting in its channels.
func (c *cell) retire() {
	c.router.Retire(c.me)
	c.bounce.Stop()
	for _, m := range channels.Drain(c.in.In) {
		c.router.Return(m, sim.Retired)
	}
}
