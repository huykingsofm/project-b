package replay

import (
	"errors"
	"fmt"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
)

type replayRun struct {
	cluster *engine.Cluster
	engines map[sim.StateID]engine.Stepper
	order   []sim.StateID
	limits  map[sim.StateID]uint64
	cur     map[sim.StateID]uint64
	cursors map[sim.StateID]*cursor
	windows map[sim.StateID]*tickWindow
	version map[sim.StateID]uint64
	configs map[uint64]*config.Snapshot
	opts    Options
	router  *replayRouter
	stable  *replayStable
	cfg     *replayConfig
	verify  *verifyLog
	div     *divergences
}

func newReplayRun(b Builder, p *Plan, opts Options) (*replayRun, error) {
	logs := p.logs
	idx, err := buildIndex(logs, b.Seed(), b.QueueSize(), p.starts)
	if err != nil {
		return nil, err
	}
	div := &divergences{}
	r := &replayRun{
		engines: map[sim.StateID]engine.Stepper{},
		limits:  p.limits,
		cur:     map[sim.StateID]uint64{},
		cursors: map[sim.StateID]*cursor{},
		windows: map[sim.StateID]*tickWindow{},
		version: map[sim.StateID]uint64{},
		configs: map[uint64]*config.Snapshot{},
		opts:    opts,
		cfg:     &replayConfig{snap: config.Defaults()},
		div:     div,
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
	r.verify = &verifyLog{windows: r.windows, pos: map[sim.StateID]int{}, div: div}
	env := &engine.Env{
		Router: r.router, Cfg: r.cfg, EventLog: r.verify, Stable: r.stable,
		Codecs: b.Codecs(), QueueSize: idx.maxQueueSize, Emitter: engine.NopEmitter{},
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
		r.engines[id] = e
		r.order = append(r.order, id)
		r.cur[id] = e.Tick()
	}
	div.name = func(id sim.StateID) string {
		if e, ok := r.engines[id]; ok {
			return fmt.Sprintf("%s %d", e.TypeName(), id)
		}
		return fmt.Sprint(id)
	}
	for _, address := range logs.Engines() {
		if _, ok := r.engines[address]; !ok {
			return nil, fmt.Errorf(
				"the log of %v is not the log of an engine of the cluster", address,
			)
		}
		src, err := logs.Open(address, p.starts[address])
		if err != nil {
			r.close()
			return nil, fmt.Errorf("the log of %v: %w", address, err)
		}
		r.cursors[address] = newCursor(src)
		if err := r.begin(address, idx); err != nil {
			r.close()
			return nil, err
		}
	}
	return r, nil
}

// begin takes an engine that was made from a checkpoint to where its log goes on from it. The
// records of the tick it starts at are in the checkpoint already, so they are not given to it, but
// for the hash, which says that the engine that was made is the one that was recorded. What the
// engine knew of the configuration, and the messages that it sent and that are not settled, are
// what the records after the tick refer to. Its state is published, since the tick that does it
// has run.
func (r *replayRun) begin(address sim.StateID, idx *index) error {
	eng := r.engines[address]
	start := r.cur[address]
	if start == 0 {
		return nil
	}
	records, err := r.cursors[address].take(start)
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
	r.version[address] = eng.ConfigVersionSeen()
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
	for _, c := range r.cursors {
		c.close()
	}
}

// load reads the records of the tick that an engine is about to run, once, and tells the registry
// which copy each read among them will find.
func (r *replayRun) load(e sim.StateID, tick uint64) (*tickWindow, error) {
	if w := r.windows[e]; w != nil && w.tick == tick {
		return w, nil
	}
	w := &tickWindow{tick: tick}
	if c := r.cursors[e]; c != nil {
		records, err := c.take(tick)
		if err != nil {
			return nil, fmt.Errorf("the log of %v: %w", e, err)
		}
		w.records = records
	}
	for _, rec := range w.records {
		if read, ok := rec.Body.(eventlog.StableRead); ok {
			r.stable.expect(readKey{e, tick, read.Source}, read.CopyTick)
			if read.CopyTick > 0 {
				w.needs = append(w.needs, replayNeed{read.Source, read.CopyTick})
			}
		}
	}
	r.windows[e] = w
	r.verify.pos[e] = 0
	return w, nil
}

// ready reports whether the engine can run its tick: every copy that tick read exists.
func (r *replayRun) ready(e sim.StateID, tick uint64) (bool, error) {
	w, err := r.load(e, tick)
	if err != nil {
		return false, err
	}
	for _, n := range w.needs {
		if _, ok := r.engines[n.source]; ok && r.cur[n.source] < n.copyTick {
			return false, nil
		}
	}
	return true, nil
}

// play runs the engines until each has run to its last tick.
func (r *replayRun) play() error {
	for {
		progressed, done := false, true
		for _, e := range r.order {
			for r.cur[e] < r.limits[e] {
				ok, err := r.ready(e, r.cur[e]+1)
				if err != nil {
					return err
				}
				if !ok {
					break
				}
				if err := r.stepEngine(e, r.cur[e]+1); err != nil {
					return err
				}
				progressed = true
			}
			if r.cur[e] < r.limits[e] {
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

// snapshotOf returns the configuration of a version, made once.
func (r *replayRun) snapshotOf(e sim.StateID, version uint64) *config.Snapshot {
	if s, ok := r.configs[version]; ok {
		return s
	}
	s := config.Defaults()
	if version != 0 {
		overrides, known := r.opts.ConfigVersions[version]
		if !known {
			r.div.add(e, "the version %d of the configuration is not given to the replay", version)
		} else if built, _, err := config.Build(version, overrides); err != nil {
			r.div.add(e, "the version %d of the configuration cannot be built: %v", version, err)
		} else {
			s = built
		}
	}
	r.configs[version] = s
	return s
}

// stepEngine gives an engine what its log says it was given at the tick, and runs the tick.
func (r *replayRun) stepEngine(e sim.StateID, tick uint64) error {
	eng := r.engines[e]
	w, err := r.load(e, tick)
	if err != nil {
		return err
	}
	for _, rec := range w.records {
		switch body := rec.Body.(type) {
		case eventlog.ConfigRead:
			r.version[e] = body.Version
		case eventlog.Command:
			if err := eng.SendBaseCommand(body.Command); err != nil {
				return fmt.Errorf("%v tick %d: %w", e, tick, err)
			}
		case eventlog.Inbound:
			if !r.router.inject(body.Message) {
				return fmt.Errorf("%v tick %d: a message could not be put in its queue", e, tick)
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
	r.cfg.snap = r.snapshotOf(e, r.version[e])
	if err := eng.Step(); err != nil {
		return err
	}
	r.cur[e] = tick
	return nil
}
