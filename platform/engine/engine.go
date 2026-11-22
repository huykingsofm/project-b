package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/binarycodec"
	"github.com/huykingsofm/project-b/shared/channels"
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
	selfNext []sim.Action[S]
	commands chan sim.Command[S]
	stable   StableStore
	emitter  Emitter
	plans    *PlanPool[S]
	tick     uint64
}

// New makes the engine of a state. The tick is its own: the caller gives the ticks with Run or
// calls Step.
func New[S sim.State](st S, env *Env, seed uint64) *Engine[S] {
	me := st.ID()
	env.ReplayLog.Describe(me, st.Type(), st.String())
	e := &Engine[S]{
		cell: newCell(
			me, env.Router, env.Cfg, env.ReplayLog, env.Stable, env.QueueSize, seed,
		), st: st,
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

// Waiting returns the action that the key names among what the engine set for the next tick and
// what waits for a later one. It is called between two ticks, by the goroutine of the engine or
// while the engine does not run.
func (e *Engine[S]) Waiting(key sim.ActionKey) (sim.Action[S], bool) {
	return sim.FindWaiting(&e.sched, e.selfNext, key)
}

// ScheduleAt sets an action to run at a tick of the engine. It is called between two ticks, by the
// goroutine of the engine or while the engine does not run.
func (e *Engine[S]) ScheduleAt(tick uint64, a sim.Action[S]) { e.sched.At(tick, a) }

// Step runs one tick of the engine:
//
//  1. read the configuration, once, and handle what came back undelivered;
//  2. build the plan: what the engine set for itself, what was scheduled for this tick, the
//     messages of other engines, the commands;
//  3. run the plan, one action after another, on the state;
//  4. settle the actions that were cancelled, and take what the actions set for the engine;
//  5. hand the events to the emitter;
//  6. publish a copy of the state;
//  7. send the messages that the actions put in the outbox.
func (e *Engine[S]) Step() {
	e.tick++
	cfg := e.snapshot(e.tick)
	ctx := &sim.Context{Tick: e.tick, Rand: e.rng, Cfg: cfg}
	e.st.SetTick(e.tick)
	e.reg.Begin(e.tick)
	e.view.begin(e.tick)
	e.settle(cfg, e.tick)

	plan := e.plans.Get(cfg)
	defer e.plans.Put(plan)
	e.planSelf(plan)
	e.planScheduled(plan)
	e.planMessages(plan)
	e.planCommands(plan)

	for {
		entry, ok := plan.Next()
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
	next, later := e.reg.TakeSelf()
	e.selfNext = sim.SettleSelf(&e.sched, next, later, e.reg.TakeCancels())

	if events := e.st.TakeEvents(); len(events) > 0 {
		e.emitter.Emit(e.me, e.tick, events)
	}
	e.stable.Publish(e.st.Clone())
	e.recordHash(cfg, e.tick, e.Hash)
	e.flush(cfg, e.tick, e.reg.Take())
	e.endTick(e.tick)
}

// planSelf puts in the plan what the engine set for itself in the last tick.
func (e *Engine[S]) planSelf(plan *sim.Plan[S]) {
	for _, a := range e.selfNext {
		plan.Append(a)
	}
	e.selfNext = nil
}

// planScheduled puts in the plan the actions that were scheduled for this tick.
func (e *Engine[S]) planScheduled(plan *sim.Plan[S]) {
	for _, a := range e.sched.Due(e.tick) {
		plan.Append(a)
	}
}

// planMessages puts in the plan the actions of the messages that other engines sent, in the order
// they arrived.
func (e *Engine[S]) planMessages(plan *sim.Plan[S]) {
	for _, a := range e.receivedActions[S](e.tick) {
		plan.Append(a)
	}
}

// planCommands takes the commands that clients sent since the last tick, writes them to the replay
// log and puts their actions in the plan.
func (e *Engine[S]) planCommands(plan *sim.Plan[S]) {
	for _, c := range channels.Drain(e.commands) {
		e.replayLog.Append(e.me, e.tick, replay.Command{Command: c})
		for _, a := range c.Translate() {
			plan.Append(a)
		}
	}
}

// Run steps the engine at every tick that arrives, until the context ends.
func (e *Engine[S]) Run(ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			e.retire()
			return
		case <-ticks:
			e.Step()
		}
	}
}

// Hash is the hash of the engine: its state, what it waits for, and what only the engine knows (see
// engineHash). A replay compares it with the one the log holds.
func (e *Engine[S]) Hash() uint64 {
	return engineHash(&e.cell, e.st.Hash(), &e.sched, e.selfNext)
}

// Checkpoint writes what the engine needs to go on from this tick. It is called between two ticks,
// by the goroutine of the engine or while the engine does not run.
func (e *Engine[S]) Checkpoint() ([]byte, error) {
	return checkpoint(&e.cell, e.tick, e.st, &e.sched, e.selfNext)
}

// Restore makes the engine of a state from a checkpoint, in the environment given, at the tick of
// the checkpoint. The state is read with the codecs of the environment. What the engine does again
// and again is an action that waits in the checkpoint like any other.
func Restore[S sim.State](data []byte, env *Env, seed uint64) (*Engine[S], error) {
	r := binarycodec.NewReader(data)
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
	rest, err := readCheckpointRest[S](env.Codecs, r, version)
	if err != nil {
		return nil, fmt.Errorf("%d: %w", id, err)
	}
	e := New(st, env, seed)
	e.tick = tick
	e.sched, e.selfNext = rest.sched, rest.self
	e.cell.restore(rest.cell)
	return e, nil
}
