package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/channels"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/logging"
	"github.com/huykingsofm/project-b/shared/rand"
)

// Noticer is implemented by a state that can tell an actor something when a message of its engine
// could not be delivered and says to notify: the OnFail Notify. A state that is not one tells no
// one.
type Noticer interface {
	Notice(actor any, key string, args ...any)
}

// Engine is the engine of a state of type S: it owns the state and is the only one that changes it.
// Every engine of a game is one of these, whatever its state is.
type Engine[S sim.State] struct {
	peer peer // what the engine has to talk to the other engines
	st   S

	rng      *rand.Rand // the random numbers of the engine and where they are
	eventLog eventlog.Log

	actionScheduler sim.ActionScheduler[S]
	stateRegistry   sim.Registry[S]
	actionBuffer    []sim.Action[S] // what the plan of a tick starts with, kept for the next tick
	planPool        *PlanPool[S]
	commands        chan sim.Command[S]
	stable          StableStore
	emitter         Emitter
	// checkpoints writes the checkpoints the engine takes at the end of a tick; nil when it takes
	// none (a replay)
	checkpoints CheckpointWriter
	tick        uint64
	// periodOffset is where the engine starts the count of a period (see dueAt): the hash of its
	// ID, so that the engines of a cluster are spread over the ticks of a period
	periodOffset uint64
}

// eventLogHashEvery is every how many ticks an engine records its hash: it costs as much as the
// state is big, so it is worked out and recorded a period of ticks apart and not at every tick. A
// replay that finds a difference at a hash runs the ticks since the one before it again to find
// where it began. The engine records it at every tick it takes a checkpoint at as well.
var eventLogHashEvery = config.NewVar("eventlog.hash_every_ticks", 20, config.Range(1, 100000))

// checkpointEvery is every how many ticks an engine takes a checkpoint, at the end of the tick: a
// replay or a recovery starts an engine from its latest checkpoint instead of from the tick 1. It
// is much rarer than the hash; each engine starts the count at a place of its own (see dueAt), so
// that the engines do not all take one at the same tick.
var checkpointEvery = config.NewVar("checkpoint.every_ticks", 600, config.Range(1, 100000000))

// ErrTickMismatch is wrapped by the error of an engine whose state is not at the tick the engine is
// at.
var ErrTickMismatch = errors.New("the tick of the state is not the tick of the engine")

// New makes the engine of a state, at the tick the state is at. The tick is its own: the caller
// gives the ticks with Run or calls Step. The log of the engine refuses a frame that does not
// follow its last, so a state that is made again for an engine with a log is at the tick of it.
func New[S sim.State](st S, env *Env, seed uint64) *Engine[S] {
	me := st.ID()
	e := &Engine[S]{
		peer: newPeer(
			me, env.Router, env.Stable.View(me, env.EventLog), env.QueueSize, env.Acks,
		), st: st, tick: st.Tick(),
		// the seed of the cluster and the ID of the engine decide the random numbers, so the same
		// inputs give the same run
		rng: rand.NewPCG(seed, uint64(me)), eventLog: env.EventLog,
		commands: make(chan sim.Command[S], env.QueueSize), stable: env.Stable,
		emitter: env.Emitter, checkpoints: env.Checkpoints, planPool: planPoolOf[S](),
		periodOffset: periodOffsetOf(me),
	}
	e.stateRegistry.Me, e.stateRegistry.Stable = me, e.peer.view
	e.peer.bounce.resend = e.peer.sendToUnsettled
	if n, ok := any(st).(Noticer); ok {
		e.peer.bounce.SetNotice(
			func(actor any, key string, args []any) { n.Notice(actor, key, args...) },
		)
	}
	return e
}

// Remove ends the engine for good, as opposed to stopping it with the context of Run: its log gets
// a last frame, of the tick after the last it ran, that holds that it was removed; what it
// published is forgotten, nothing more is delivered to it, and what is waiting in its queue comes
// back to its senders. The engine is not used after.
func (e *Engine[S]) Remove() {
	e.tick++
	e.eventLog.Append(e.peer.me, e.tick, eventlog.Retired{})
	e.eventLog.EndTick(e.peer.me, e.tick)
	e.RemoveAlreadyRecorded()
}

// RemoveAlreadyRecorded ends the engine for good as Remove does, without writing the last frame to
// its event log: the log already ends with it, as in an engine that was removed before a restart.
// The engine does not run a tick, so the copy of its state that the other engines read goes, and
// what they read after is nothing.
func (e *Engine[S]) RemoveAlreadyRecorded() {
	e.stable.Remove(e.peer.me)
	e.peer.retire()
}

// Send gives the engine a command, without waiting. It reports false when the queue is full.
func (e *Engine[S]) Send(c sim.Command[S]) bool {
	if c == nil {
		logging.Error("a nil command was sent and is not taken", "engine", e.peer.me)
		return false
	}
	c = c.Clone() // the caller keeps its command and may change it: the engine keeps a copy
	select {
	case e.commands <- c:
		return true
	default:
		return false
	}
}

// SendBaseCommand gives the engine a command that was read without its type, as a log holds it. It
// returns an error for a command of another type of state, and for a full queue.
func (e *Engine[S]) SendBaseCommand(c sim.BaseCommand) error {
	typed, ok := c.(sim.Command[S])
	if !ok {
		return fmt.Errorf(
			"%T is not a command of the engine %d, whose state is of the type %d",
			c, e.peer.me, sim.StateTypeOf[S](),
		)
	}
	if !e.Send(typed) {
		return errors.New("the queue of commands is full")
	}
	return nil
}

// State returns the state of the engine. Only the goroutine that runs the engine may use it; other
// goroutines read what the engine published.
func (e *Engine[S]) State() S { return e.st }

// StateID is the ID of the state of the engine, which is also its address.
func (e *Engine[S]) StateID() sim.StateID { return e.peer.me }

// Tick returns the last tick that the engine ran.
func (e *Engine[S]) Tick() uint64 { return e.tick }

// Type is the type of the state of the engine.
func (e *Engine[S]) Type() sim.StateType { return sim.StateTypeOf[S]() }

// TypeName is the name of the type of the state of the engine.
func (e *Engine[S]) TypeName() string { return e.Type().String() }

// Scheduler returns what the engine waits for, to be read: the actions set for a tick to come, the
// next one included. It is read between two ticks, by the goroutine of the engine or while the
// engine does not run.
func (e *Engine[S]) Scheduler() sim.ReadOnlyScheduler[S] { return &e.actionScheduler }

// ScheduleAt sets an action to run at a tick of the engine. It is called between two ticks, by the
// goroutine of the engine or while the engine does not run.
func (e *Engine[S]) ScheduleAt(tick uint64, a sim.Action[S]) { e.actionScheduler.At(tick, a) }

// Step runs one tick of the engine:
//
//  1. read the configuration, once, and handle what came back undelivered;
//  2. build the plan, in one go: what was scheduled for this tick (what the engine
//     set for itself included), the messages of other engines, the commands;
//  3. run the plan, one action after another, on the state;
//  4. settle the actions that were cancelled, and take what the actions set for the engine;
//  5. hand the events to the emitter;
//  6. publish a copy of the state;
//  7. let go of the messages that their receivers acked, send the messages that the actions put
//     in the outbox, and send again the ones whose ack is late;
//  8. take a checkpoint, when it is due, and hand it to be written out of the goroutine;
//  9. write the hash of the engine to the event log, when it is due or a checkpoint was due,
//     and close the tick for the log.
//
// It first takes the state to the next tick, and returns an error, having done nothing else, when
// the state does not then say the tick that the engine is at: a state whose NextTick is wrong can
// no longer be run, nor replayed, and the cluster stops.
func (e *Engine[S]) Step() error {
	e.tick++
	e.st.NextTick()
	if got := e.st.Tick(); got != e.tick {
		return fmt.Errorf(
			"the engine %d: %w: the state is at tick %d, and the engine at %d",
			e.peer.me, ErrTickMismatch, got, e.tick,
		)
	}
	e.stateRegistry.Begin(e.tick)
	e.peer.view.begin(e.tick)
	cfg := e.stateRegistry.Config()
	ctx := &sim.Context{Tick: e.tick, Rand: e.rng.Rand}
	e.peer.handleReturnedMessages(e.tick, e.eventLog)

	e.actionBuffer = e.actionBuffer[:0]
	e.planScheduled()
	e.planMessages()
	e.planCommands()

	plan := e.planPool.Get()
	defer e.planPool.Put(plan)
	plan.Build(e.actionBuffer)
	// the plan holds the actions: the list does not keep them for the next tick
	clear(e.actionBuffer)

	maxDepth := sim.MaxDepth.Get(cfg)
	for {
		entry, ok := plan.Next(maxDepth)
		if !ok {
			break
		}
		entry.Action().Apply(ctx, e.st, &e.stateRegistry, plan)
	}
	for _, entry := range plan.Cancelled() {
		if c, ok := entry.Action().(sim.Cancellable[S]); ok {
			c.ApplyCancel(ctx, e.st, &e.stateRegistry)
		}
	}
	for _, t := range e.stateRegistry.TakeSelf() {
		e.actionScheduler.At(t.At, t.Action)
	}

	if events := e.st.TakeEvents(); len(events) > 0 {
		e.emitter.Emit(e.peer.me, e.tick, events)
	}
	published := e.st.Clone().(S)
	e.stable.Publish(published)
	e.peer.releaseAckedMessages()
	e.peer.flushUnsettledMessages(e.tick, e.stateRegistry.Take(), e.eventLog)
	checkpointDue := e.dueAt(e.tick, uint64(checkpointEvery.Get(cfg)))
	if checkpointDue {
		e.offerCheckpoint(published)
	}
	if checkpointDue || e.dueAt(e.tick, uint64(eventLogHashEvery.Get(cfg))) {
		e.eventLog.Append(e.peer.me, e.tick, eventlog.Hash{Value: e.Hash(sim.NewHasher())})
	}
	e.eventLog.EndTick(e.peer.me, e.tick)
	e.peer.publishAcks(e.eventLog)
	return nil
}

// dueAt reports whether a period of every ticks comes round at the tick, for this engine: the
// hash and the checkpoint are each done once a period. The count starts at a place of its own
// (periodOffset), so that the engines of a cluster do not all do the work of a period at the same
// tick and make a peak of work there. It depends on the configuration and the tick alone, so a
// replay does it at the same ticks.
func (e *Engine[S]) dueAt(tick, every uint64) bool {
	return (tick+e.periodOffset%every)%every == 0
}

// offerCheckpoint takes the checkpoint of the engine at the end of the tick and hands it to the
// checkpoint writer, which encodes it and writes the file on a goroutine of its own. The state is
// the copy that was just published, which no one changes; the rest is copied here, before the next
// tick changes it.
func (e *Engine[S]) offerCheckpoint(published S) {
	if e.checkpoints == nil {
		return
	}
	e.checkpoints.Offer(e.peer.me, e.tick, e.checkpointWith(published).Encode)
}

// Checkpoint takes the checkpoint of the engine as it is: a copy that shares nothing with it. It is
// called between two ticks, by the goroutine of the engine or while the engine does not run.
func (e *Engine[S]) Checkpoint() EngineCheckpoint[S] {
	return e.checkpointWith(e.st.Clone().(S))
}

// checkpointWith is the checkpoint of the engine with the state given, which is a copy of the
// state of the engine that no one changes: what the engine waits for, what only it knows and what
// its peer knows are copied with their Clone.
func (e *Engine[S]) checkpointWith(st S) EngineCheckpoint[S] {
	return EngineCheckpoint[S]{
		ID: e.peer.me, Tick: e.tick, State: st, ActionScheduler: e.actionScheduler.Clone(),
		Rand: e.rng.Clone(), Peer: e.peer.data.Clone(),
	}
}

// SentMessages are the messages the engine sent and has not had acked: the engine sends them again
// until their receivers say they hold them safe. The messages are the engine's own.
func (e *Engine[S]) SentMessages() []sim.Message { return e.peer.data.unsettledMessages.Sent() }

// EncodeCheckpoint takes the checkpoint of the engine as it is and encodes it: what Checkpoint
// gives, written, for a cluster that holds its engines without their state type.
func (e *Engine[S]) EncodeCheckpoint() ([]byte, error) { return e.Checkpoint().Encode() }

// PublishState publishes a copy of the state as it is. A tick does it at its end; an engine made
// from a checkpoint calls it before its first tick, so that what the others read of it is there.
func (e *Engine[S]) PublishState() { e.stable.Publish(e.st.Clone().(S)) }

// planScheduled adds the actions that were scheduled for this tick, among them what the engine set
// for itself in the last tick.
func (e *Engine[S]) planScheduled() {
	e.actionBuffer = append(e.actionBuffer, e.actionScheduler.Due(e.tick)...)
}

// planMessages adds the actions of the messages that other engines sent, in the order they arrived.
func (e *Engine[S]) planMessages() {
	e.actionBuffer = append(e.actionBuffer, e.peer.receivedActions[S](e.tick, e.eventLog)...)
}

// planCommands takes the commands that clients sent since the last tick, writes them to the replay
// log and adds their actions.
func (e *Engine[S]) planCommands() {
	for _, c := range channels.Drain(e.commands) {
		e.eventLog.Append(e.peer.me, e.tick, eventlog.Command{Command: c})
		e.actionBuffer = append(e.actionBuffer, c.Translate()...)
	}
}

// Run steps the engine at every tick that arrives, until the context ends or a tick fails. It
// returns the error of the tick that failed, and nil when the context ended.
func (e *Engine[S]) Run(ctx context.Context, ticks <-chan time.Time) error {
	for {
		select {
		case <-ctx.Done():
			e.peer.retire()
			return nil
		case <-ticks:
			if err := e.Step(); err != nil {
				e.peer.retire()
				return err
			}
		}
	}
}

// Hash is the hash of the engine: of everything a checkpoint of it holds that the engine goes on
// from: its state, what it waits for, what only it knows (the place of its random numbers), and
// what its peer knows (the number of its messages and
// the last it took from each sender, and the messages it holds to send again). A restore that
// differs from the engine it came from in any of them has another hash, and a replay finds it.
func (e *Engine[S]) Hash(h sim.Hasher) uint64 {
	h.U64(e.st.Hash())
	e.actionScheduler.Hash(&h)
	var w binary.Writer
	if err := e.encodeOwnData(&w); err != nil {
		h.String("an engine whose random numbers cannot be written")
		logging.Error(
			"the hash of the engine cannot cover its random numbers",
			"engine", e.peer.me, "err", err,
		)
	}
	h.Bytes(w.B)
	if err := e.peer.data.Hash(&h); err != nil {
		logging.Error(
			"the hash of the engine cannot cover what its peer holds",
			"engine", e.peer.me, "err", err,
		)
	}
	return h.Sum()
}

// encodeOwnData writes what only the engine knows besides its state, its scheduler and its peer:
// the place of its random numbers.
func (e *Engine[S]) encodeOwnData(w *binary.Writer) error {
	if err := e.rng.Encode(w); err != nil {
		return fmt.Errorf("its random numbers: %w", err)
	}
	return nil
}

// periodOffsetOf is where an engine starts the count of a period: the hash of its ID. The IDs of
// the engines of one type follow one another, and the type is in their high bits; the hash spreads
// them over a period whatever it is.
func periodOffsetOf(id sim.StateID) uint64 {
	h := sim.NewHasher()
	h.U64(uint64(id))
	return h.Sum()
}
