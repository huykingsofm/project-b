package engine

import (
	"context"
	"fmt"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
	"github.com/huykingsofm/project-b/server/internal/shared/logging"
	"time"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/command"
	worldstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world"
	"github.com/huykingsofm/project-b/server/internal/shared/channels"
)

// WorldEngine is the engine of the world: it owns the state of the world, which summarizes the
// zones and manages the private messages between players, and is the only one that changes it.
type WorldEngine struct {
	cell
	st       *worldstate.State
	sched    action.Scheduler[action.WorldAction]
	reg      action.WorldRegistry
	selfNext []action.WorldAction
	commands chan command.WorldCommand
	stable   StableRegistry
	emitter  WorldEmitter
	tick     uint64
}

// NewWorldEngine makes the engine of the world.
func NewWorldEngine(st *worldstate.State, env *Env, seed uint64) *WorldEngine {
	me := message.WorldAddress(st.ID)
	e := &WorldEngine{
		cell: newCell(
			me, env.Router, env.Cfg, env.ReplayLog, env.Stable, env.QueueSize, seed,
		), st: st,
		commands: make(
			chan command.WorldCommand, env.QueueSize,
		), stable: env.Stable, emitter: env.WorldEmitter,
	}
	e.reg.Common = action.Common{Me: me, Graph: env.Graph, Stable: e.view}
	return e
}

// Send gives the world a command, without waiting. It reports false when the queue is full.
func (e *WorldEngine) Send(c command.WorldCommand) bool {
	if c == nil {
		logging.Error("a nil command was sent and is not taken", "engine", "world")
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

// State returns the state of the world. Only the goroutine that runs the world may use it.
func (e *WorldEngine) State() *worldstate.State { return e.st }

// Step runs one tick of the world, in the same seven steps as a zonelayer (see
// ZoneLayerEngine.Step).
func (e *WorldEngine) Step() {
	e.tick++
	cfg := e.snapshot(e.tick)
	ctx := &action.Context{Tick: e.tick, Rand: e.rng, Cfg: cfg}
	e.st.SetTick(e.tick)
	e.reg.Begin(e.tick)
	e.view.begin(e.tick)
	e.settle(cfg, e.tick)

	plan := worldPlans.Get(cfg)
	defer worldPlans.Put(plan)
	e.planSelf(plan)
	e.planScheduled(plan)
	e.planMessages(plan)
	e.planCommands(plan)

	for {
		entry, ok := plan.Next()
		if !ok {
			break
		}
		entry.Action().ApplyWorld(ctx, e.st, &e.reg, plan)
	}
	next, later := e.reg.TakeSelf()
	e.selfNext = action.SettleSelf(&e.sched, next, later, e.reg.TakeCancels())

	if events := e.st.TakeEvents(); len(events) > 0 {
		e.emitter.EmitWorld(e.me, e.tick, events)
	}
	e.stable.PublishWorld(e.st.Clone())
	e.recordHash(cfg, e.tick, e.hash)
	e.flush(cfg, e.tick, e.reg.Take())
	e.endTick(e.tick)
}

// planSelf puts in the plan what the engine set for itself in the last tick.
func (e *WorldEngine) planSelf(plan *action.Plan[action.WorldAction]) {
	for _, a := range e.selfNext {
		plan.Append(a)
	}
	e.selfNext = nil
}

// planScheduled puts in the plan the actions that were scheduled for this tick.
func (e *WorldEngine) planScheduled(plan *action.Plan[action.WorldAction]) {
	for _, a := range e.sched.Due(e.tick) {
		plan.Append(a)
	}
}

// planMessages puts in the plan the actions of the messages that other engines sent, in the order
// they arrived.
func (e *WorldEngine) planMessages(plan *action.Plan[action.WorldAction]) {
	for _, a := range e.receivedActions[action.WorldAction](e.tick) {
		plan.Append(a)
	}
}

// planCommands takes the commands that clients sent since the last tick, writes them to the replay
// log and puts their actions in the plan.
func (e *WorldEngine) planCommands(plan *action.Plan[action.WorldAction]) {
	for _, c := range channels.Drain(e.commands) {
		e.replayLog.Append(e.me, e.tick, replay.Command{Command: c})
		for _, a := range c.TranslateWorld() {
			plan.Append(a)
		}
	}
}

// Run steps the world at every tick that arrives, until the context ends.
func (e *WorldEngine) Run(ctx context.Context, ticks <-chan time.Time) {
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

// hash is the hash of the engine: its state and what it waits for.
func (e *WorldEngine) hash() uint64 { return engineHash(e.st.Hash(), &e.sched, e.selfNext) }

// Checkpoint writes what the engine needs to go on from this tick. It is called between two ticks,
// by the goroutine of the engine or while the engine does not run.
func (e *WorldEngine) Checkpoint() ([]byte, error) {
	encodeState := func(w *binarycodec.Writer) error {
		e.st.Encode(w)
		return nil
	}
	return checkpoint(&e.cell, e.tick, encodeState, &e.sched, e.selfNext)
}

// RestoreWorldEngine makes the engine of the world from a checkpoint, in the environment given, at
// the tick of the checkpoint.
func RestoreWorldEngine(data []byte, env *Env, seed uint64) (*WorldEngine, error) {
	r := binarycodec.NewReader(data)
	version, address, tick, err := readCheckpointStart(r, message.ScopeWorld)
	if err != nil {
		return nil, err
	}
	st, err := worldstate.DecodeState(r, version)
	if err != nil {
		return nil, fmt.Errorf("%v: the state: %w", address, err)
	}
	if message.WorldAddress(st.ID) != address {
		return nil, fmt.Errorf("%v: the state is of the world %d", address, st.ID)
	}
	e := NewWorldEngine(st, env, seed)
	e.tick = tick
	if e.selfNext, err = readCheckpointRest(r, version, &e.cell, &e.sched); err != nil {
		return nil, fmt.Errorf("%v: %w", address, err)
	}
	return e, nil
}
