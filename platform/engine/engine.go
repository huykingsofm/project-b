package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/channels"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/logging"
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
	cell
	st       S
	sched    sim.Scheduler[S]
	reg      sim.Registry[S]
	start    []sim.Action[S] // what the plan of a tick starts with, kept for the next tick
	commands chan sim.Command[S]
	stable   StableStore
	emitter  Emitter
	plans    *PlanPool[S]
	tick     uint64
}

// ErrTickMismatch is wrapped by the error of an engine whose state is not at the tick the engine is
// at.
var ErrTickMismatch = errors.New("the tick of the state is not the tick of the engine")

// New makes the engine of a state, at the tick the state is at. The tick is its own: the caller
// gives the ticks with Run or calls Step.
func New[S sim.State](st S, env *Env, seed uint64) *Engine[S] {
	me := st.ID()
	env.ReplayLog.Describe(me, st.String())
	e := &Engine[S]{
		cell: newCell(
			me, env.Router, env.Cfg, env.ReplayLog, env.Stable, env.QueueSize, seed,
		), st: st, tick: st.Tick(),
		commands: make(chan sim.Command[S], env.QueueSize), stable: env.Stable,
		emitter: env.Emitter, plans: planPoolOf[S](),
	}
	e.reg.Me, e.reg.Stable = me, e.view
	if n, ok := any(st).(Noticer); ok {
		e.bounce.notice = func(actor any, key string, args []any) { n.Notice(actor, key, args...) }
	}
	return e
}

// Send gives the engine a command, without waiting. It reports false when the queue is full.
func (e *Engine[S]) Send(c sim.Command[S]) bool {
	if c == nil {
		logging.Error("a nil command was sent and is not taken", "engine", e.me)
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
			c, e.me, sim.StateTypeOf[S](),
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

// Tick returns the last tick that the engine ran.
func (e *Engine[S]) Tick() uint64 { return e.tick }

// Type is the type of the state of the engine.
func (e *Engine[S]) Type() sim.StateType { return sim.StateTypeOf[S]() }

// TypeName is the name of the type of the state of the engine.
func (e *Engine[S]) TypeName() string { return e.st.String() }

// Scheduler returns what the engine waits for, to be read: the actions set for a tick to come, the
// next one included. It is read between two ticks, by the goroutine of the engine or while the
// engine does not run.
func (e *Engine[S]) Scheduler() sim.ReadOnlyScheduler[S] { return &e.sched }

// ScheduleAt sets an action to run at a tick of the engine. It is called between two ticks, by the
// goroutine of the engine or while the engine does not run.
func (e *Engine[S]) ScheduleAt(tick uint64, a sim.Action[S]) { e.sched.At(tick, a) }

// Step runs one tick of the engine:
//
//  1. read the configuration, once, and handle what came back undelivered;
//  2. build the plan, in one go: what was scheduled for this tick (what the engine
//     set for itself included), the messages of other engines, the commands;
//  3. run the plan, one action after another, on the state;
//  4. settle the actions that were cancelled, and take what the actions set for the engine;
//  5. hand the events to the emitter;
//  6. publish a copy of the state;
//  7. send the messages that the actions put in the outbox.
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
			e.me, ErrTickMismatch, got, e.tick,
		)
	}
	cfg := e.snapshot(e.tick)
	ctx := &sim.Context{Tick: e.tick, Rand: e.rng, Cfg: cfg}
	e.reg.Begin(e.tick)
	e.view.begin(e.tick)
	e.settle(cfg, e.tick)

	e.start = e.start[:0]
	e.planScheduled()
	e.planMessages()
	e.planCommands()

	plan := e.plans.Get()
	defer e.plans.Put(plan)
	plan.Build(e.start)
	clear(e.start) // the plan holds the actions: the list does not keep them for the next tick

	maxDepth := sim.MaxDepth.Get(cfg)
	for {
		entry, ok := plan.Next(maxDepth)
		if !ok {
			break
		}
		entry.Action().Apply(ctx, e.st, &e.reg, plan)
	}
	for _, entry := range plan.Cancelled() {
		if c, ok := entry.Action().(sim.Cancellable[S]); ok {
			c.ApplyCancel(ctx, e.st, &e.reg)
		}
	}
	for _, t := range e.reg.TakeSelf() {
		e.sched.At(t.At, t.Action)
	}

	if events := e.st.TakeEvents(); len(events) > 0 {
		e.emitter.Emit(e.me, e.tick, events)
	}
	e.stable.Publish(e.st.Clone())
	e.recordHash(cfg, e.tick, e.Hash)
	e.flush(cfg, e.tick, e.reg.Take())
	e.endTick(e.tick)
	return nil
}

// planScheduled adds the actions that were scheduled for this tick, among them what the engine set
// for itself in the last tick.
func (e *Engine[S]) planScheduled() {
	e.start = append(e.start, e.sched.Due(e.tick)...)
}

// planMessages adds the actions of the messages that other engines sent, in the order they arrived.
func (e *Engine[S]) planMessages() {
	e.start = append(e.start, e.receivedActions[S](e.tick)...)
}

// planCommands takes the commands that clients sent since the last tick, writes them to the replay
// log and adds their actions.
func (e *Engine[S]) planCommands() {
	for _, c := range channels.Drain(e.commands) {
		e.replayLog.Append(e.me, e.tick, replay.Command{Command: c})
		e.start = append(e.start, c.Translate()...)
	}
}

// Run steps the engine at every tick that arrives, until the context ends or a tick fails. It
// returns the error of the tick that failed, and nil when the context ended.
func (e *Engine[S]) Run(ctx context.Context, ticks <-chan time.Time) error {
	for {
		select {
		case <-ctx.Done():
			e.retire()
			return nil
		case <-ticks:
			if err := e.Step(); err != nil {
				e.retire()
				return err
			}
		}
	}
}

// Hash is the hash of the engine: its state, what it waits for, and what only the engine knows (see
// engineHash). A replay compares it with the one the log holds.
func (e *Engine[S]) Hash() uint64 {
	return e.cell.engineHash(e.st.Hash(), &e.sched)
}

// Checkpoint writes what the engine needs to go on from this tick. It is called between two ticks,
// by the goroutine of the engine or while the engine does not run.
func (e *Engine[S]) Checkpoint() ([]byte, error) {
	return e.cell.checkpoint(e.tick, e.st, &e.sched)
}

// Restore makes the engine of a state from a checkpoint, in the environment given, at the tick of
// the checkpoint. The state is read with the codecs of the environment. What the engine does again
// and again is an action that waits in the checkpoint like any other.
func Restore[S sim.State](data []byte, env *Env, seed uint64) (*Engine[S], error) {
	r := binary.NewReader(data)
	version, id, tick, err := readCheckpointStart(r, sim.StateTypeOf[S]())
	if err != nil {
		return nil, err
	}
	decoded, err := env.Codecs.DecodeStateOfType(sim.StateTypeOf[S](), r, version)
	if err != nil {
		return nil, fmt.Errorf("%d: the state: %w", id, err)
	}
	st, ok := decoded.(S)
	if !ok {
		return nil, fmt.Errorf("%d: the state is a %T, not the one asked for", id, decoded)
	}
	if st.ID() != id {
		return nil, fmt.Errorf("%d: the state is of the engine %d", id, st.ID())
	}
	if st.Tick() != tick {
		return nil, fmt.Errorf(
			"%d: %w: the state is at tick %d, and the checkpoint at %d",
			id, ErrTickMismatch, st.Tick(), tick,
		)
	}
	rest, err := env.readCheckpointRest[S](r, version)
	if err != nil {
		return nil, fmt.Errorf("%d: %w", id, err)
	}
	e := New(st, env, seed)
	e.tick = tick
	e.sched = rest.sched
	e.cell.restore(rest.cell)
	return e, nil
}
