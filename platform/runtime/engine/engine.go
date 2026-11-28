package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"time"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/runtime/router"
	"github.com/huykingsofm/project-b/platform/runtime/stable"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/channels"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/logging"
	"github.com/huykingsofm/project-b/shared/rand"
)

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

// ErrCommandQueueFull is the error of a command that an engine could not take now: its queue holds
// as many as it can.
var ErrCommandQueueFull = errors.New("the queue of commands is full")

// ErrTickMismatch is wrapped by the error of an engine whose state is not at the tick the engine is
// at.
var ErrTickMismatch = errors.New("the tick of the state is not the tick of the engine")

// retirementRequest says who asked an engine to retire, and at which of its ticks.
type retirementRequest struct {
	by   sim.StateID
	tick uint64
}

// Engine is the engine of a state of type S: it owns the state and is the only one that changes it.
// Every engine of a game is one of these, whatever its state is.
type Engine[S sim.State] struct {
	// who the engine is, and where it is in time
	peer         peer
	st           S
	tick         uint64
	rng          *rand.Rand
	periodOffset uint64 // where it starts the count of a period (see dueAt): the hash of its ID

	// what a tick runs: the actions that wait, what an action asks of the cluster, and what the
	// plan of a tick starts with (kept for the next tick)
	actionScheduler sim.ActionScheduler[S]
	stateRegistry   sim.ActionRegistry[S]
	actionBuffer    []sim.Action[S]
	planPool        *PlanPool[S]
	commands        chan sim.Command[S]

	// where it writes what it does; checkpointWriter is nil when it takes no checkpoint (a replay)
	eventLog         eventlog.Log
	stableStore      stable.Store
	emitter          Emitter
	checkpointWriter CheckpointWriter

	// how an engine that a request made is begun: initRecord begins its log, written at its first
	// tick (nil for the coordinator engine and for an engine that a checkpoint made again);
	// startedRecord is written at the tick after, when its creator took the response (Adopt);
	// acknowledgementsToPublish are the responses that the tick took, published when the tick is
	// done
	initRecord                *eventlog.Init
	startedRecord             *eventlog.Started
	acknowledgementsToPublish []router.ResponseAcknowledgement

	// how it ends: retirementSignal asks Run to retire it, retirementAsked says by whom, and
	// retiredTick is the tick of the last frame of its log, which holds that it retired (0 until
	// it did)
	retirementSignal chan struct{}
	retirementAsked  atomic.Pointer[retirementRequest]
	retiredTick      atomic.Uint64
}

// NewEngine makes the engine with the ID of a state, at the tick the state is at. The ID is the
// engine's: the state does not hold it. The tick is its own: the caller gives the ticks with Run or
// calls Step. The log of the engine refuses a frame that does not follow its last, so a state that
// is made again for an engine with a log is at the tick of it.
func NewEngine[S sim.State](st S, me sim.StateID, env *Environment) *Engine[S] {
	e := newEngine(st, me, env)
	e.peer.joinMessageRouter()
	return e
}

// newEngine makes the engine as NewEngine does, but not reachable by the router: the engine that
// the cluster makes by a request joins it when the engine that asked has taken the response.
func newEngine[S sim.State](st S, me sim.StateID, env *Environment) *Engine[S] {
	e := &Engine[S]{
		peer: newPeer(
			me, env.Router, env.StableStore.View(me, env.EventLog), env.QueueSize, env.AckBoard,
			env.DirectiveRouter,
		), st: st, tick: st.Tick(),
		// the seed of the cluster and the ID of the engine decide the random numbers, so the same
		// inputs give the same run
		rng: rand.NewPCG(env.Seed, uint64(me)), eventLog: env.EventLog,
		commands: make(chan sim.Command[S], env.QueueSize), stableStore: env.StableStore,
		emitter: env.Emitter, checkpointWriter: env.CheckpointWriter, planPool: planPoolOf[S](),
		periodOffset:     periodOffsetOf(me),
		retirementSignal: make(chan struct{}, 1),
	}
	e.stateRegistry.Me, e.stateRegistry.Stable = me, e.peer.view
	e.peer.bounce.resend = e.peer.sendToUnsettled
	return e
}

// Retire ends the engine for good, as opposed to stopping it with the context of Run: its log gets
// a last frame, of the tick after the last it ran, that holds that it retired, and then what it
// registered is cleaned up. The engine is not used after. An engine on its own stops only by this:
// a tick that fails stops the whole cluster.
func (e *Engine[S]) Retire() {
	e.tick++
	retired := eventlog.Retired{}
	if asked := e.retirementAsked.Load(); asked != nil {
		retired = eventlog.Retired{By: asked.by, ByTick: asked.tick}
	}
	e.eventLog.Append(e.peer.me, e.tick, retired)
	e.eventLog.EndTick(e.peer.me, e.tick)
	e.retiredTick.Store(e.tick)
	e.cleanUpRegistrations()
}

// Adopt tells an engine that a request made, and that has run its tick 1 only, that the engine that
// asked took the response to its request at the tick given: the engine becomes reachable by the
// router, and writes the record of it (Started) at its next tick, which depends on that tick. The
// goroutine that calls it is the one that runs the engine from then on.
func (e *Engine[S]) Adopt(creator sim.StateID, responseTick uint64) {
	e.startedRecord = &eventlog.Started{Creator: creator, ResponseTick: responseTick}
	e.peer.joinMessageRouter()
}

// RetireIfRequested retires the engine if it was asked to (RequestRetirement) and says whether it
// did. It is for a cluster that runs its engines by Step: an engine that Run runs takes the request
// itself, between two ticks. It must be called between two ticks.
func (e *Engine[S]) RetireIfRequested() bool {
	select {
	case <-e.retirementSignal:
		e.Retire()
		return true
	default:
		return false
	}
}

// RequestRetirement asks the engine to retire, at the end of the tick it runs or before the next
// one: Run does it. by is the engine that asked and byTick the tick of that engine at which it
// asked, which the last frame of the log of the engine says; 0 and 0 when the engine asked for
// itself. It does not wait and may be called from any goroutine; a second request, or one to an
// engine that has no Run, does nothing.
func (e *Engine[S]) RequestRetirement(by sim.StateID, byTick uint64) {
	e.retirementAsked.CompareAndSwap(nil, &retirementRequest{by: by, tick: byTick})
	select {
	case e.retirementSignal <- struct{}{}:
	default:
	}
}

// WaitsForDirectiveResponse says that the engine made a request with the number and has not taken
// the
// response to it. It is for a recovery, which decides what to do with an engine that the request
// made: it must be called between two ticks.
func (e *Engine[S]) WaitsForDirectiveResponse(request uint64) bool {
	pending := e.peer.data.pendingDirectiveRequests
	return slices.ContainsFunc(pending, func(r sim.DirectiveRequest) bool {
		return r.ID == request
	})
}

// IsRetired says whether the engine retired, and the tick of the last frame of its log, which holds
// that it did; 0 and false for an engine that did not retire.
func (e *Engine[S]) IsRetired() (retiredTick uint64, retired bool) {
	retiredTick = e.retiredTick.Load()
	return retiredTick, retiredTick > 0
}

// SendCommand gives the engine a command, without waiting. It reports false when the queue is full.
func (e *Engine[S]) SendCommand(c sim.Command[S]) bool {
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
	if !e.SendCommand(typed) {
		return ErrCommandQueueFull
	}
	return nil
}

// State returns the state of the engine. Only the goroutine that runs the engine may use it; other
// goroutines read what the engine published.
func (e *Engine[S]) State() S { return e.st }

// StateID is the ID of the engine, which is also its address and the ID of its state.
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
//  0. take the answers of the cluster to the requests of the engine;
//  1. read the configuration, once, and handle what came back undelivered;
//  2. build the plan, in one go: what was scheduled for this tick (what the engine
//     set for itself included), the messages of other engines, the commands;
//  3. run the plan, one action after another, on the state;
//  4. settle the actions that were cancelled, and take what the actions set for the engine;
//  5. hand the events to the emitter;
//  6. publish a copy of the state;
//  7. hand the cluster the requests to make and to remove engines, let go of the messages that
//     their receivers acked, send the messages that the actions put
//     in the outbox, and send again the ones whose ack is late;
//  8. take a checkpoint, when it is due, and hand it to be written out of the goroutine;
//  9. write the hash of the engine to the event log, when it is due or a checkpoint was due,
//     and close the tick for the log.
//
// It first takes the state to the next tick, and returns an error, having done nothing else, when
// the state does not then say the tick that the engine is at: a state whose AdvanceTick is wrong
// can no longer be run, nor replayed, and the cluster stops.
func (e *Engine[S]) Step() error {
	e.tick++
	e.st.AdvanceTick()
	if got := e.st.Tick(); got != e.tick {
		return fmt.Errorf(
			"the engine %d: %w: the state is at tick %d, and the engine at %d",
			e.peer.me, ErrTickMismatch, got, e.tick,
		)
	}
	if e.initRecord != nil {
		e.eventLog.Append(e.peer.me, e.tick, *e.initRecord)
		e.initRecord = nil
	}
	if e.startedRecord != nil {
		e.eventLog.Append(e.peer.me, e.tick, *e.startedRecord)
		e.startedRecord = nil
	}
	e.stateRegistry.Begin(e.tick)
	e.peer.view.BeginTick(e.tick)
	cfg := e.stateRegistry.Config()
	ctx := &sim.Context{Self: e.peer.me, Tick: e.tick, Rand: e.rng.Rand}
	e.takeDirectiveResponses(ctx)
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
	e.stableStore.Publish(e.peer.me, published)
	for _, a := range e.acknowledgementsToPublish {
		e.peer.directives.PublishResponseAcknowledgement(a)
	}
	e.acknowledgementsToPublish = e.acknowledgementsToPublish[:0]
	e.peer.publishDirectives(e.tick, e.stateRegistry.TakeDirectiveRequests())
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
	e.peer.publishMessageAcks(e.eventLog)
	return nil
}

// Checkpoint takes the checkpoint of the engine as it is: a copy that shares nothing with it. It is
// called between two ticks, by the goroutine of the engine or while the engine does not run.
func (e *Engine[S]) Checkpoint() Checkpoint[S] {
	return e.checkpointWith(e.st.Clone().(S))
}

// GetSentMessages returns the messages the engine sent and has not had acked: the engine sends
// them again until their receivers say they hold them safe. The messages are the engine's own.
func (e *Engine[S]) GetSentMessages() []sim.Message { return e.peer.data.unsettledMessages.Sent() }

// EncodeCheckpoint takes the checkpoint of the engine as it is and encodes it: what Checkpoint
// gives, written, for a cluster that holds its engines without their state type.
func (e *Engine[S]) EncodeCheckpoint() ([]byte, error) { return e.Checkpoint().Encode() }

// PublishState publishes a copy of the state as it is. A tick does it at its end; an engine made
// from a checkpoint calls it before its first tick, so that what the others read of it is there.
func (e *Engine[S]) PublishState() { e.stableStore.Publish(e.peer.me, e.st.Clone().(S)) }

// Run steps the engine at every tick that arrives, until the context ends, a tick fails, or the
// engine is asked to retire. It returns the error of the tick that failed, and nil otherwise. An
// engine never stops on its own except by retiring: a failure, or a stop, is that of the whole
// cluster.
func (e *Engine[S]) Run(ctx context.Context, ticks <-chan time.Time) error {
	for {
		select {
		case <-ctx.Done(): // a stop, not a removal: the engine goes on after a restart
			e.peer.leaveRouters()
			return nil
		case <-e.retirementSignal: // ends the engine for good: its log ends with the record of it
			e.Retire()
			return nil
		case <-ticks:
			if err := e.Step(); err != nil {
				e.peer.leaveRouters()
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
	h.U64(uint64(e.peer.me))
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

// cleanUpRegistrations takes back what newEngine registered, and writes nothing to the event log:
// what the engine published is forgotten, so what the other engines read of it after is nothing,
// nothing more is delivered to it, and what is waiting in its queue comes back to its senders. It
// is called by Retire, and by Make for an engine that could not be made.
func (e *Engine[S]) cleanUpRegistrations() {
	e.stableStore.Remove(e.peer.me)
	e.peer.leaveRouters()
}

// buildInitRecord makes the record of the tick 1 of the engine, which it writes at its first tick:
// the
// engine that asked for it to be made, the tick of that engine at which it asked and the number of
// the request, and the engine as it is, at the tick it was made at, written as a checkpoint. It is
// for an engine that a request made: a replay makes it from this record.
func (e *Engine[S]) buildInitRecord(creator sim.StateID, creatorTick, directiveID uint64) error {
	snapshot, err := e.EncodeCheckpoint()
	if err != nil {
		return fmt.Errorf("the engine %d cannot be written as it starts: %w", e.peer.me, err)
	}
	e.initRecord = &eventlog.Init{
		Creator: creator, CreatorTick: creatorTick, DirectiveID: directiveID, Snapshot: snapshot,
	}
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
	if e.checkpointWriter == nil {
		return
	}
	e.checkpointWriter.Offer(e.peer.me, e.tick, e.checkpointWith(published).Encode)
}

// checkpointWith is the checkpoint of the engine with the state given, which is a copy of the
// state of the engine that no one changes: what the engine waits for, what only it knows and what
// its peer knows are copied with their Clone.
func (e *Engine[S]) checkpointWith(st S) Checkpoint[S] {
	return Checkpoint[S]{
		ID: e.peer.me, Tick: e.tick, State: st, ActionScheduler: e.actionScheduler.Clone(),
		Rand: e.rng.Clone(), Peer: e.peer.data.Clone(),
	}
}

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

// encodeOwnData writes what only the engine knows besides its state, its scheduler and its peer:
// the place of its random numbers.
func (e *Engine[S]) encodeOwnData(w *binary.Writer) error {
	if err := e.rng.Encode(w); err != nil {
		return fmt.Errorf("its random numbers: %w", err)
	}
	return nil
}

// takeDirectiveResponses takes the answers of the cluster, at the start of the tick, before
// everything
// else: each is written to the event log, since a replay cannot make it again, and handed to the
// state, which changes as an action does. The answer to a request that has none waiting is not an
// answer: it is dropped without a trace.
func (e *Engine[S]) takeDirectiveResponses(ctx *sim.Context) {
	for _, d := range e.peer.directives.TakeResponses(e.peer.me) {
		pending := e.peer.data.pendingDirectiveRequests
		i := slices.IndexFunc(pending, func(r sim.DirectiveRequest) bool { return r.ID == d.ID })
		if i < 0 {
			continue
		}
		e.peer.data.pendingDirectiveRequests = slices.Delete(pending, i, i+1)
		e.eventLog.Append(e.peer.me, e.tick, eventlog.Directive{Response: d})
		if d.Kind == sim.DirectiveCreateEngine && d.Failure == "" {
			acknowledgement := router.ResponseAcknowledgement{
				From: e.peer.me, Request: d.ID, Tick: e.tick,
			}
			e.acknowledgementsToPublish = append(e.acknowledgementsToPublish, acknowledgement)
		}
		handler, ok := any(e.st).(sim.DirectiveHandler[S])
		if !ok {
			logging.Warn(
				"datafix: the state does not take the answer of the cluster",
				"engine", e.peer.me, "directive", d.ID, "kind", d.Kind, "failure", d.Failure,
			)
			continue
		}
		handler.HandleDirective(ctx, &e.stateRegistry, d)
	}
}

// periodOffsetOf is where an engine starts the count of a period: the hash of its ID. The IDs of
// the engines of one type follow one another, and the type is in their high bits; the hash spreads
// them over a period whatever it is.
func periodOffsetOf(id sim.StateID) uint64 {
	h := sim.NewHasher()
	h.U64(uint64(id))
	return h.Sum()
}
