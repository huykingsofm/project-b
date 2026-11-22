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
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/internal/shared/channels"
)

// ZoneLayerEngine is the engine of a zonelayer: it owns the state of the zonelayer and is the only
// one that changes it.
type ZoneLayerEngine struct {
	cell
	st       *zonelayerstate.State
	sched    action.Scheduler[action.ZoneLayerAction]
	reg      action.ZoneLayerRegistry
	selfNext []action.ZoneLayerAction
	commands chan command.ZoneLayerCommand
	stable   StableRegistry
	emitter  ZoneLayerEmitter
	tick     uint64
}

// NewZoneLayerEngine makes the engine of a zonelayer. The tick is its own: the caller gives the
// ticks with Run or calls Step.
func NewZoneLayerEngine(st *zonelayerstate.State, env *Env, seed uint64) *ZoneLayerEngine {
	me := message.ZoneLayerAddress(st.ID)
	e := &ZoneLayerEngine{
		cell: newCell(
			me, env.Router, env.Cfg, env.ReplayLog, env.Stable, env.QueueSize, seed,
		), st: st,
		commands: make(
			chan command.ZoneLayerCommand, env.QueueSize,
		), stable: env.Stable, emitter: env.ZoneLayerEmitter,
	}
	e.reg.Common = action.Common{Me: me, Graph: env.Graph, Stable: e.view}
	e.bounce.notice = func(who any, key string, args []any) {
		if characterID, ok := who.(state.CharacterID); ok {
			e.st.NoticeToCharacter(characterID, key, args...)
		}
	}
	return e
}

// Send gives the zonelayer a command, without waiting. It reports false when the queue is full.
func (e *ZoneLayerEngine) Send(c command.ZoneLayerCommand) bool {
	if c == nil {
		logging.Error("a nil command was sent and is not taken", "engine", "zonelayer")
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

// State returns the state of the zonelayer. Only the goroutine that runs the zonelayer may use it;
// other goroutines read what the zonelayer published.
func (e *ZoneLayerEngine) State() *zonelayerstate.State { return e.st }

// Step runs one tick of the zonelayer:
//
//  1. read the configuration, once, and handle what came back undelivered;
//  2. build the plan: what the zonelayer set for itself, what was scheduled for this tick, the
//     messages of other engines, the commands;
//  3. run the plan, one action after another, on the state;
//  4. settle the actions that were cancelled, and take what the actions set for the zonelayer;
//  5. hand the events to the emitter;
//  6. publish a copy of the state;
//  7. send the messages that the actions put in the outbox.
func (e *ZoneLayerEngine) Step() {
	e.tick++
	cfg := e.snapshot(e.tick)
	ctx := &action.Context{Tick: e.tick, Rand: e.rng, Cfg: cfg}
	e.st.SetTick(e.tick)
	e.reg.Begin(e.tick)
	e.view.begin(e.tick)
	e.settle(cfg, e.tick)

	plan := zoneLayerPlans.Get(cfg)
	defer zoneLayerPlans.Put(plan)
	e.planSelf(plan)
	e.planScheduled(plan)
	e.planMessages(plan)
	e.planCommands(plan)

	for {
		entry, ok := plan.Next()
		if !ok {
			break
		}
		entry.Action().ApplyZoneLayer(ctx, e.st, &e.reg, plan)
	}
	for _, entry := range plan.Cancelled() {
		if c, ok := entry.Action().(action.ZoneLayerCancellableAction); ok {
			c.ApplyZoneLayerCancel(ctx, e.st, &e.reg)
		}
	}
	next, later := e.reg.TakeSelf()
	e.selfNext = action.SettleSelf(&e.sched, next, later, e.reg.TakeCancels())

	if events := e.st.TakeEvents(); len(events) > 0 {
		e.emitter.EmitZoneLayer(e.me, e.tick, events)
	}
	e.stable.PublishZoneLayer(e.st.Clone())
	e.recordHash(cfg, e.tick, e.hash)
	e.flush(cfg, e.tick, e.reg.Take())
	e.endTick(e.tick)
}

// planSelf puts in the plan what the engine set for itself in the last tick.
func (e *ZoneLayerEngine) planSelf(plan *action.Plan[action.ZoneLayerAction]) {
	for _, a := range e.selfNext {
		plan.Append(a)
	}
	e.selfNext = nil
}

// planScheduled puts in the plan the actions that were scheduled for this tick.
func (e *ZoneLayerEngine) planScheduled(plan *action.Plan[action.ZoneLayerAction]) {
	for _, a := range e.sched.Due(e.tick) {
		plan.Append(a)
	}
}

// planMessages puts in the plan the actions of the messages that other engines sent, in the order
// they arrived.
func (e *ZoneLayerEngine) planMessages(plan *action.Plan[action.ZoneLayerAction]) {
	for _, a := range e.receivedActions[action.ZoneLayerAction](e.tick) {
		plan.Append(a)
	}
}

// planCommands takes the commands that clients sent since the last tick, writes them to the replay
// log and puts their actions in the plan.
func (e *ZoneLayerEngine) planCommands(plan *action.Plan[action.ZoneLayerAction]) {
	for _, c := range channels.Drain(e.commands) {
		e.replayLog.Append(e.me, e.tick, replay.Command{Command: c})
		for _, a := range c.TranslateZoneLayer() {
			plan.Append(a)
		}
	}
}

// Run steps the zonelayer at every tick that arrives, until the context ends.
func (e *ZoneLayerEngine) Run(ctx context.Context, ticks <-chan time.Time) {
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
func (e *ZoneLayerEngine) hash() uint64 { return engineHash(e.st.Hash(), &e.sched, e.selfNext) }

// Checkpoint writes what the engine needs to go on from this tick. It is called between two ticks,
// by the goroutine of the engine or while the engine does not run.
func (e *ZoneLayerEngine) Checkpoint() ([]byte, error) {
	return checkpoint(&e.cell, e.tick, e.st.Encode, &e.sched, e.selfNext)
}

// RestoreZoneLayerEngine makes the engine of a zonelayer from a checkpoint, in the environment
// given, at the tick of the checkpoint. What the zonelayer does again and again, healing its
// characters, is an action that waits in the checkpoint like any other; EnsureRegen checks that the
// zonelayer is made to do it as the cluster says.
func RestoreZoneLayerEngine(data []byte, env *Env, seed uint64) (*ZoneLayerEngine, error) {
	r := binarycodec.NewReader(data)
	version, address, tick, err := readCheckpointStart(r, message.ScopeZoneLayer)
	if err != nil {
		return nil, err
	}
	st, err := zonelayerstate.DecodeState(r, version)
	if err != nil {
		return nil, fmt.Errorf("%v: the state: %w", address, err)
	}
	if message.ZoneLayerAddress(st.ID) != address {
		return nil, fmt.Errorf("%v: the state is of the zonelayer %d", address, st.ID)
	}
	e := NewZoneLayerEngine(st, env, seed)
	e.tick = tick
	if e.selfNext, err = readCheckpointRest(r, version, &e.cell, &e.sched); err != nil {
		return nil, fmt.Errorf("%v: %w", address, err)
	}
	return e, nil
}

// EnsureRegen makes the zonelayer heal its characters every every ticks, which one action does: it
// heals and sets itself again. When no such action waits, the first is set for every ticks from
// now; when one waits, it must be the one asked for. One that waits with another period is an
// error, because a period that changed while the cluster ran would change what a replay makes, and
// so is one that waits when the period is 0, which means no healing.
func (e *ZoneLayerEngine) EnsureRegen(every uint64) error {
	key := action.PlanKey{ID: action.ZoneLayerCharacterRegenActionID}
	waiting, found := action.FindWaiting(&e.sched, e.selfNext, key)
	if !found {
		if every > 0 {
			e.sched.At(e.tick+every, &action.ZoneLayerCharacterRegenAction{Every: every})
		}
		return nil
	}
	regen, ok := waiting.(*action.ZoneLayerCharacterRegenAction)
	if !ok || regen.Every != every {
		return fmt.Errorf(
			"%v: the zonelayer heals every %d ticks and the cluster says every %d",
			e.me, regenEveryOf(waiting), every,
		)
	}
	return nil
}

// regenEveryOf is the period of an action that heals, or 0.
func regenEveryOf(a action.ZoneLayerAction) uint64 {
	if regen, ok := a.(*action.ZoneLayerCharacterRegenAction); ok {
		return regen.Every
	}
	return 0
}
