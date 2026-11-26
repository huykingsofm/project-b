package replay

import (
	"errors"
	"fmt"

	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/stablemap"
)

// engineRun is what a replay holds of one engine: the engine, how far it has to run and has run,
// and where its log is.
type engineRun struct {
	engine engine.Stepper
	// limitTick is the last tick to run: the limit of the engine in the plan.
	limitTick uint64
	// lastRanTick is the last tick that has run.
	lastRanTick uint64
	// tickReader reads its log a tick at a time; nil when it has none.
	tickReader *tickReader
	// loaded says that the records of the tick to run next are loaded: false before, and again
	// after the tick has run.
	loaded bool
	// tickRecords are the records that the log holds for the tick to run next, in order.
	tickRecords []eventlog.Record
	// copiesRead are the copies that the reads among tickRecords find.
	copiesRead []copyKey
	// recordsMade is how many of tickRecords the engine has made while it runs the tick.
	recordsMade int
}

type replayRun struct {
	cluster *engine.Cluster
	// engines are the engines of the cluster, in the order that Step runs them in.
	engines stablemap.Map[sim.StateID, *engineRun]
	router  *replayRouter
	stable  *replayStable
	verify  *verifyLog
	div     *divergences
	// retired is the engines that the replay removed, in the order it did.
	retired []sim.StateID
}

func newReplayRun(b Builder, p *Plan) (*replayRun, error) {
	logs := p.logs
	idx, err := buildIndex(logs, b.Seed(), b.QueueSize(), p.starts)
	if err != nil {
		return nil, err
	}
	div := &divergences{}
	r := &replayRun{
		div: div,
	}
	r.router = &replayRouter{
		Router:             engine.NewRouter(),
		inboundMessageHash: idx.inboundMessageHash,
		msgsToBeReturned:   idx.msgsToBeReturned,
		sent:               map[msgKey]sim.Message{},
		refuse:             map[msgKey]sim.DeliveryFailure{},
		div:                div,
	}
	r.stable = newReplayStable(idx.copyReads)
	r.verify = &verifyLog{engines: &r.engines, div: div}
	env := &engine.Env{
		Router: r.router, EventLog: r.verify, Stable: r.stable,
		Codecs: b.Codecs(), QueueSize: idx.maxQueueSize, Emitter: engine.NopEmitter{},
		Acks: engine.NewAckBoard(),
	}
	var checkpoints engine.CheckpointSource = engine.NoCheckpoints{}
	if p.checkpoints != nil {
		checkpoints = p.checkpoints
	}
	if r.cluster, err = b.BuildIn(env, checkpoints); err != nil {
		return nil, err
	}
	for _, e := range r.cluster.Engines() {
		id := e.StateID()
		if start := p.starts[id]; e.Tick() != start {
			return nil, fmt.Errorf(
				"the engine %v is made at the tick %d and the replay starts it at %d",
				id, e.Tick(), start,
			)
		}
		r.engines.Set(id, &engineRun{engine: e, limitTick: p.limits[id], lastRanTick: e.Tick()})
	}
	div.name = func(id sim.StateID) string {
		if er, ok := r.engines.Get(id); ok {
			return fmt.Sprintf("%s %d", er.engine.TypeName(), id)
		}
		return fmt.Sprint(id)
	}
	for _, address := range logs.Engines() {
		er, ok := r.engines.Get(address)
		if !ok {
			r.close()
			return nil, fmt.Errorf(
				"the log of %v is not the log of an engine of the cluster", address,
			)
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
	for _, m := range eng.SentMessages() {
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
		er.tickRecords = records
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
	if !r.cluster.Remove(id) {
		return fmt.Errorf("%v tick %d: the engine was removed and the cluster does not have it",
			id, tick)
	}
	r.retired = append(r.retired, id)
	er.lastRanTick = tick
	er.loaded = false
	return nil
}
