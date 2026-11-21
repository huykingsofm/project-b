package engine

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/command"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/internal/shared/channels"
)

// ZoneLayerEngine is the engine of a zonelayer: it owns the state of the zonelayer and is the only one that
// changes it.
type ZoneLayerEngine struct {
	cell
	st       *zonelayerstate.State
	sched    action.Scheduler[action.ZoneLayerAction]
	reg      action.ZoneLayerRegistry
	selfNext []action.ZoneLayerAction
	commands chan command.ZoneLayerCommand
	stable   *StableStateRegistry
	emitter  ZoneLayerEmitter
	tick     uint64
	rng      *rand.Rand
}

// NewZoneLayerEngine makes the engine of a zonelayer. The tick and the cron are its own: the caller gives
// the ticks with Run or calls Step.
func NewZoneLayerEngine(st *zonelayerstate.State, env *Env, seed uint64) *ZoneLayerEngine {
	me := action.ZoneLayerAddress(st.ID)
	e := &ZoneLayerEngine{
		cell: newCell(me, env.Router, env.Cfg, env.ReplayLog, env.Stable, env.QueueSize), st: st,
		commands: make(chan command.ZoneLayerCommand, env.QueueSize), stable: env.Stable, emitter: env.ZoneLayerEmitter, rng: newRand(seed, me),
	}
	e.reg.Common = action.Common{Me: me, Graph: env.Graph, Stable: e.view}
	e.bounce.notice = func(who any, key string, args []any) {
		if characterID, ok := who.(state.CharacterID); ok {
			e.st.NoticeToCharacter(characterID, key, args...)
		}
	}
	return e
}

// AddCron makes actions run every every ticks.
func (e *ZoneLayerEngine) AddCron(every uint64, actions ...action.ZoneLayerAction) {
	e.sched.AddCron(every, actions...)
}

// Send gives the zonelayer a command, without waiting. It reports false when the queue is full.
func (e *ZoneLayerEngine) Send(c command.ZoneLayerCommand) bool {
	c = command.Isolate(c) // the caller keeps its command and may change it: the engine keeps a copy
	select {
	case e.commands <- c:
		return true
	default:
		return false
	}
}

// State returns the state of the zonelayer. Only the goroutine that runs the zonelayer may use it; other
// goroutines read what the zonelayer published.
func (e *ZoneLayerEngine) State() *zonelayerstate.State { return e.st }

// Step runs one tick of the zonelayer:
//
//  1. read the configuration, once, and decide on what came back undelivered;
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
	e.settle(cfg)

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
	e.selfNext = next
	for _, t := range later {
		e.sched.At(t.At, t.Action)
	}

	if events := e.st.TakeEvents(); len(events) > 0 {
		e.emitter.EmitZoneLayer(e.me, e.tick, events)
	}
	e.stable.PublishZoneLayer(e.st.Clone())
	e.recordHash(cfg, e.tick, e.st.Hash)
	e.flush(cfg, e.reg.Take())
	e.endTick(e.tick)
}

// planSelf puts in the plan what the engine set for itself in the last tick.
func (e *ZoneLayerEngine) planSelf(plan *action.Plan[action.ZoneLayerAction]) {
	for _, a := range e.selfNext {
		plan.Append(a)
	}
	e.selfNext = nil
}

// planScheduled puts in the plan the actions that were scheduled for this tick, and the crons that are due.
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

// planCommands takes the commands that clients sent since the last tick, writes them to the replay log
// and puts their actions in the plan.
func (e *ZoneLayerEngine) planCommands(plan *action.Plan[action.ZoneLayerAction]) {
	for _, c := range channels.Drain(e.commands) {
		e.replayLog.ZoneLayerCommand(e.me, e.tick, c)
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
