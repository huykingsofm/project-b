package replay

import (
	"errors"
	"fmt"
	"slices"

	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/runtime/cluster"
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/runtime/router"
	"github.com/huykingsofm/project-b/platform/runtime/stable"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/stablemap"
)

// runEngineHandle is what the run needs of an engine of the replay cluster: to run a tick and give
// it its inputs, to compare what it did with its log, and to publish its state. A handle of the
// cluster is converted to it once, when the run holds the engine.
type runEngineHandle interface {
	StateID() sim.StateID
	TypeName() string
	Tick() uint64
	Step() error
	Hash(h sim.Hasher) uint64
	GetSentMessages() []sim.Message
	PublishState()
	SendBaseCommand(c sim.BaseCommand) error
	RequestRetirement(by sim.StateID, byTick uint64)
	RetireIfRequested() bool
}

var _ runEngineHandle = (*engine.Engine[*sim.CoordinatorState])(nil)

// engineRun is what a replay holds of one engine: the engine, how far it has to run and has run,
// and where its log is.
type engineRun struct {
	// the engine, the last tick to run (its limit in the plan) and the last tick that has run
	engine      runEngineHandle
	limitTick   uint64
	lastRanTick uint64

	// its log, read a tick at a time (nil when it has none), whether the records of the tick to run
	// next are loaded (false before, and again after the tick has run), those records in order, and
	// the copies that the reads among them find
	tickReader  *tickReader
	loaded      bool
	tickRecords []eventlog.Record
	copiesRead  []copyKey

	// how many of the records the engine has made while it runs the tick
	recordsMade int
}

type replayRun struct {
	// the cluster, and its engines in the order that Step runs them in
	cluster *cluster.Cluster
	engines stablemap.Map[sim.StateID, *engineRun]

	// what the engines talk through: the message router, the stable copies, and the responses that
	// the logs say the engines took, until they take them (the requests and the acknowledgements
	// of the engines are thrown away)
	router     *replayRouter
	stable     *replayStable
	directives *router.DirectiveRouter

	// what it checks and reports: the log that the run is compared with, the divergences, and the
	// engines that the replay removed, in the order it did
	verify  *verifyLog
	div     *divergences
	retired []sim.StateID
}

func newReplayRun(o engine.Options, p *RecoveryPlan) (*replayRun, error) {
	logs := p.logs
	idx, err := buildIndex(logs, o.Seed, o.QueueSize, p.starts)
	if err != nil {
		return nil, err
	}
	div := &divergences{}
	r := &replayRun{
		div: div,
	}
	r.router = &replayRouter{
		Router:             router.NewRouter(),
		inboundMessageHash: idx.inboundMessageHash,
		msgsToBeReturned:   idx.msgsToBeReturned,
		sent:               map[messageKey]sim.Message{},
		refuse:             map[messageKey]sim.DeliveryFailure{},
		div:                div,
	}
	r.directives = router.NewDirectiveRouter()
	r.stable = newReplayStable(idx.copyReads)
	r.verify = &verifyLog{engines: &r.engines, div: div}
	env := &engine.Environment{
		Router: r.router, EventLog: r.verify, StableStore: r.stable,
		Codecs: o.Codecs, EngineRegistry: o.Engines, Seed: o.Seed, QueueSize: idx.maxQueueSize,
		Emitter: engine.NopEmitter{}, AckBoard: stable.NewAckBoard(),
		DirectiveRouter: r.directives,
	}
	var coordinatorCheckpoint []byte
	if p.starts[sim.CoordinatorStateID] > 0 {
		var err error
		if coordinatorCheckpoint, err = r.checkpointOf(p, sim.CoordinatorStateID); err != nil {
			return nil, err
		}
	}
	if r.cluster, err = cluster.NewReplayCluster(env, coordinatorCheckpoint); err != nil {
		return nil, err
	}
	for _, handle := range r.cluster.Engines() {
		e := handle.(runEngineHandle)
		r.engines.Set(e.StateID(), &engineRun{
			engine: e, limitTick: p.limits[e.StateID()], lastRanTick: e.Tick(),
		})
	}
	for _, address := range logs.Engines() {
		if _, ok := r.engines.Get(address); ok || p.limits[address] == 0 {
			continue
		}
		e, err := r.makeEngineFromLog(p, idx, address)
		if err != nil {
			return nil, err
		}
		r.engines.Set(address, &engineRun{
			engine: e, limitTick: p.limits[address], lastRanTick: e.Tick(),
		})
	}
	div.name = func(id sim.StateID) string {
		if er, ok := r.engines.Get(id); ok {
			return fmt.Sprintf("%s %d", er.engine.TypeName(), id)
		}
		return fmt.Sprint(id)
	}
	for _, address := range logs.Engines() {
		er, ok := r.engines.Get(address)
		if !ok { // a log with no tick that is kept: nothing to run
			continue
		}
		src, err := logs.Open(address, p.starts[address])
		if err != nil {
			r.close()
			return nil, fmt.Errorf("the log of %v: %w", address, err)
		}
		er.tickReader = newTickReader(src)
		if err := r.begin(address, er, idx); err != nil {
			r.close()
			return nil, err
		}
	}
	return r, nil
}

// begin takes an engine that was made from a checkpoint to where its log goes on from it. The
// records of the tick it starts at are in the checkpoint already, so they are not given to it, but
// for the hash, which says that the engine that was made is the one that was recorded. The
// messages that it sent and that are not settled are what the records after the tick refer to. Its
// state is published, since the tick that does it has run.
func (r *replayRun) begin(address sim.StateID, er *engineRun, idx *index) error {
	eng := er.engine
	start := er.lastRanTick
	if start == 0 {
		return nil
	}
	records, err := er.tickReader.take(start)
	if err != nil {
		return fmt.Errorf("the log of %v: %w", address, err)
	}
	logged, found := uint64(0), false
	for _, rec := range records {
		if h, ok := rec.Body.(eventlog.Hash); ok {
			logged, found = h.Value, true
		}
	}
	switch got := eng.Hash(sim.NewHasher()); {
	case !found:
		r.div.add(address,
			"the log has no hash at the tick %d, where the checkpoint was taken", start)
	case got != logged:
		r.div.add(address, "the engine made from the checkpoint of the tick %d has the hash %d, "+
			"the log has %d", start, got, logged)
	}
	for _, m := range eng.GetSentMessages() {
		if key := keyOf(m.Head()); idx.msgsToBeReturned[key] {
			r.router.sent[key] = m
		}
	}
	eng.PublishState()
	return nil
}

// close lets go of the logs.
func (r *replayRun) close() {
	for _, er := range r.engines.All() {
		if er.tickReader != nil {
			er.tickReader.close()
		}
	}
}

// ticks is the last tick that each engine has run.
func (r *replayRun) ticks() map[sim.StateID]uint64 {
	out := map[sim.StateID]uint64{}
	for id, er := range r.engines.All() {
		out[id] = er.lastRanTick
	}
	return out
}

// load reads the records of the tick that an engine is about to run, once, and tells the registry
// which copy each read among them will find.
func (r *replayRun) load(id sim.StateID, er *engineRun) error {
	if er.loaded {
		return nil
	}
	tick := er.lastRanTick + 1
	er.tickRecords, er.copiesRead, er.recordsMade = nil, nil, 0
	if er.tickReader != nil {
		records, err := er.tickReader.take(tick)
		if err != nil {
			return fmt.Errorf("the log of %v: %w", id, err)
		}
		// the engine was made from the record that begins its log: it does not make it again
		er.tickRecords = slices.DeleteFunc(records, func(rec eventlog.Record) bool {
			switch rec.Body.(type) {
			case eventlog.Init, eventlog.Started:
				return true
			}
			return false
		})
	}
	for _, rec := range er.tickRecords {
		if read, ok := rec.Body.(eventlog.StableRead); ok {
			r.stable.expect(readKey{id, tick, read.Source}, read.CopyTick)
			if read.CopyTick > 0 {
				er.copiesRead = append(er.copiesRead, copyKey{read.Source, read.CopyTick})
			}
		}
	}
	er.loaded = true
	return nil
}

// ready reports whether the engine can run its next tick: every copy that tick read exists.
func (r *replayRun) ready(id sim.StateID, er *engineRun) (bool, error) {
	if err := r.load(id, er); err != nil {
		return false, err
	}
	for _, c := range er.copiesRead {
		if source, ok := r.engines.Get(c.source); ok && source.lastRanTick < c.tick {
			return false, nil
		}
	}
	return true, nil
}

// play runs the engines until each has run to its last tick.
func (r *replayRun) play() error {
	for {
		progressed, done := false, true
		for id, er := range r.engines.All() {
			for er.lastRanTick < er.limitTick {
				ok, err := r.ready(id, er)
				if err != nil {
					return err
				}
				if !ok {
					break
				}
				if err := r.stepEngine(id, er); err != nil {
					return err
				}
				progressed = true
			}
			if er.lastRanTick < er.limitTick {
				done = false
			}
		}
		if done {
			return nil
		}
		if !progressed {
			return errors.New(
				"the engines wait for each other for ever: a tick reads a copy of an engine " +
					"that cannot run so far",
			)
		}
	}
}

// stepEngine gives an engine what its log says it was given at its next tick, and runs the tick.
func (r *replayRun) stepEngine(id sim.StateID, er *engineRun) error {
	eng := er.engine
	tick := er.lastRanTick + 1
	if err := r.load(id, er); err != nil {
		return err
	}
	if len(er.tickRecords) > 0 {
		if _, removed := er.tickRecords[0].Body.(eventlog.Retired); removed {
			return r.removeEngine(id, er, tick)
		}
	}
	for _, rec := range er.tickRecords {
		switch body := rec.Body.(type) {
		case eventlog.Command:
			if err := eng.SendBaseCommand(body.Command); err != nil {
				return fmt.Errorf("%v tick %d: %w", id, tick, err)
			}
		case eventlog.Directive:
			if !r.directives.PublishResponse(id, body.Response) {
				return fmt.Errorf("%v tick %d: a response could not be given to it", id, tick)
			}
		case eventlog.Inbound:
			if !r.router.inject(body.Message) {
				return fmt.Errorf("%v tick %d: a message could not be put in its queue", id, tick)
			}
		case eventlog.Returned:
			key := keyOf(body.Header)
			if m, ok := r.router.sent[key]; ok { // sent in an earlier tick: it comes back now
				delete(r.router.sent, key)
				r.router.Return(m, body.Reason)
			} else { // it is sent in this tick, and refused at once
				r.router.refuse[key] = body.Reason
			}
		}
	}
	if err := eng.Step(); err != nil {
		return err
	}
	// the log says which engines were made and what they were answered: the requests are not served
	r.directives.TakeRequests()
	r.directives.TakeResponseAcknowledgements()
	er.lastRanTick = tick
	er.loaded = false // the records of the next tick are loaded when it is asked for
	return nil
}

// removeEngine removes the engine, as its log says it was at the tick: the frame of that tick holds
// the record of the removal and no other (the index checks it), and the engine runs no tick.
func (r *replayRun) removeEngine(id sim.StateID, er *engineRun, tick uint64) error {
	if len(er.tickRecords) != 1 {
		return fmt.Errorf("%v tick %d: the engine was removed and the tick has %d records",
			id, tick, len(er.tickRecords))
	}
	retired := er.tickRecords[0].Body.(eventlog.Retired)
	er.engine.RequestRetirement(retired.By, retired.ByTick)
	if !er.engine.RetireIfRequested() {
		return fmt.Errorf("%v tick %d: the engine was removed and did not retire", id, tick)
	}
	r.cluster.Forget(id)
	r.retired = append(r.retired, id)
	er.lastRanTick = tick
	er.loaded = false
	return nil
}

// checkpointOf is the bytes of the checkpoint that the engine starts from.
func (r *replayRun) checkpointOf(p *RecoveryPlan, id sim.StateID) ([]byte, error) {
	data, ok, err := p.checkpoints.Checkpoint(id)
	if err != nil || !ok {
		return nil, fmt.Errorf(
			"the checkpoint of %v at the tick %d: %v (found %v)", id, p.starts[id], err, ok,
		)
	}
	return data, nil
}

// makeEngineFromLog makes the engine whose log is at the address: from the checkpoint it starts
// from, or,
// when it starts at the tick 0, from the record that begins its log, which holds the engine as it
// was made. A log that does not begin with that record and has no checkpoint to start from is an
// error.
func (r *replayRun) makeEngineFromLog(
	p *RecoveryPlan, idx *index, address sim.StateID,
) (runEngineHandle, error) {
	var data []byte
	if p.starts[address] > 0 {
		checkpointData, err := r.checkpointOf(p, address)
		if err != nil {
			return nil, err
		}
		data = checkpointData
	} else {
		init, ok := idx.inits[address]
		if !ok {
			return nil, fmt.Errorf(
				"the log of %v does not begin with the record of how the engine was made, and no "+
					"checkpoint starts it", address,
			)
		}
		data = init.Snapshot
	}
	handle, err := r.cluster.AddEngineByCheckpoint(data)
	if err != nil {
		return nil, err
	}
	return handle.(runEngineHandle), nil
}
