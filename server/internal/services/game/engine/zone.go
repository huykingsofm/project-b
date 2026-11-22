package engine

import (
	"context"
	"fmt"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
	"github.com/huykingsofm/project-b/server/internal/shared/logging"
	"time"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/command"
	zonestate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone"
	"github.com/huykingsofm/project-b/server/internal/shared/channels"
)

// ZoneEngine is the engine of a zone: it owns the state of the zone, which holds its map and knows
// its zonelayers, and is the only one that changes it.
type ZoneEngine struct {
	cell
	st       *zonestate.State
	sched    action.Scheduler[action.ZoneAction]
	reg      action.ZoneRegistry
	selfNext []action.ZoneAction
	commands chan command.ZoneCommand
	stable   StableRegistry
	emitter  ZoneEmitter
	tick     uint64
}

// NewZoneEngine makes the engine of a zone.
func NewZoneEngine(st *zonestate.State, env *Env, seed uint64) *ZoneEngine {
	me := message.ZoneAddress(st.ID)
	e := &ZoneEngine{
		cell: newCell(
			me, env.Router, env.Cfg, env.ReplayLog, env.Stable, env.QueueSize, seed,
		), st: st,
		commands: make(
			chan command.ZoneCommand, env.QueueSize,
		), stable: env.Stable, emitter: env.ZoneEmitter,
	}
	e.reg.Common = action.Common{Me: me, Graph: env.Graph, Stable: e.view}
	return e
}

// Send gives the zone a command, without waiting. It reports false when the queue is full.
func (e *ZoneEngine) Send(c command.ZoneCommand) bool {
	if c == nil {
		logging.Error("a nil command was sent and is not taken", "engine", "zone")
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

// State returns the state of the zone. Only the goroutine that runs the zone may use it.
func (e *ZoneEngine) State() *zonestate.State { return e.st }

// Step runs one tick of the zone, in the same seven steps as a zonelayer (see
// ZoneLayerEngine.Step).
func (e *ZoneEngine) Step() {
	e.tick++
	cfg := e.snapshot(e.tick)
	ctx := &action.Context{Tick: e.tick, Rand: e.rng, Cfg: cfg}
	e.st.SetTick(e.tick)
	e.reg.Begin(e.tick)
	e.view.begin(e.tick)
	e.settle(cfg, e.tick)

	plan := zonePlans.Get(cfg)
	defer zonePlans.Put(plan)
	e.planSelf(plan)
	e.planScheduled(plan)
	e.planMessages(plan)
	e.planCommands(plan)

	for {
		entry, ok := plan.Next()
		if !ok {
			break
		}
		entry.Action().ApplyZone(ctx, e.st, &e.reg, plan)
	}
	next, later := e.reg.TakeSelf()
	e.selfNext = action.SettleSelf(&e.sched, next, later, e.reg.TakeCancels())

	if events := e.st.TakeEvents(); len(events) > 0 {
		e.emitter.EmitZone(e.me, e.tick, events)
	}
	e.stable.PublishZone(e.st.Clone())
	e.recordHash(cfg, e.tick, e.hash)
	e.flush(cfg, e.tick, e.reg.Take())
	e.endTick(e.tick)
}

// planSelf puts in the plan what the engine set for itself in the last tick.
func (e *ZoneEngine) planSelf(plan *action.Plan[action.ZoneAction]) {
	for _, a := range e.selfNext {
		plan.Append(a)
	}
	e.selfNext = nil
}

// planScheduled puts in the plan the actions that were scheduled for this tick.
func (e *ZoneEngine) planScheduled(plan *action.Plan[action.ZoneAction]) {
	for _, a := range e.sched.Due(e.tick) {
		plan.Append(a)
	}
}

// planMessages puts in the plan the actions of the messages that other engines sent, in the order
// they arrived.
func (e *ZoneEngine) planMessages(plan *action.Plan[action.ZoneAction]) {
	for _, a := range e.receivedActions[action.ZoneAction](e.tick) {
		plan.Append(a)
	}
}

// planCommands takes the commands that clients sent since the last tick, writes them to the replay
// log and puts their actions in the plan.
func (e *ZoneEngine) planCommands(plan *action.Plan[action.ZoneAction]) {
	for _, c := range channels.Drain(e.commands) {
		e.replayLog.Append(e.me, e.tick, replay.Command{Command: c})
		for _, a := range c.TranslateZone() {
			plan.Append(a)
		}
	}
}

// Run steps the zone at every tick that arrives, until the context ends.
func (e *ZoneEngine) Run(ctx context.Context, ticks <-chan time.Time) {
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
func (e *ZoneEngine) hash() uint64 {
	return engineHash(&e.cell, e.st.Hash(), &e.sched, e.selfNext)
}

// Checkpoint writes what the engine needs to go on from this tick. It is called between two ticks,
// by the goroutine of the engine or while the engine does not run.
func (e *ZoneEngine) Checkpoint() ([]byte, error) {
	return checkpoint(&e.cell, e.tick, e.st.Encode, &e.sched, e.selfNext)
}

// RestoreZoneEngine makes the engine of a zone from a checkpoint, in the environment given, at the
// tick of the checkpoint. The map of the zone is the one that tilesOf gives for its ID.
func RestoreZoneEngine(
	data []byte, env *Env, seed uint64, tilesOf func(zoneID state.ZoneID) *tilemap.TileMap,
) (*ZoneEngine, error) {
	r := binarycodec.NewReader(data)
	version, address, tick, err := readCheckpointStart(r, message.ScopeZone)
	if err != nil {
		return nil, err
	}
	st, err := zonestate.DecodeState(r, version, tilesOf)
	if err != nil {
		return nil, fmt.Errorf("%v: the state: %w", address, err)
	}
	if message.ZoneAddress(st.ID) != address {
		return nil, fmt.Errorf("%v: the state is of the zone %d", address, st.ID)
	}
	rest, err := readCheckpointRest[action.ZoneAction](r, version)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", address, err)
	}
	e := NewZoneEngine(st, env, seed)
	e.tick = tick
	e.sched, e.selfNext = rest.sched, rest.self
	e.cell.restore(rest.cell)
	return e, nil
}
