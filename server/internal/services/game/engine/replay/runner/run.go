package runner

import (
	"errors"
	"fmt"

	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
)

type replayRun struct {
	cluster *engine.Cluster
	engines map[message.Address]Engine
	order   []message.Address
	byTick  map[message.Address]map[uint64][]replay.Record
	last    map[message.Address]uint64
	cur     map[message.Address]uint64
	needs   map[message.Address]map[uint64][]replayNeed
	version map[message.Address]uint64
	configs map[uint64]*config.Snapshot
	opts    Options
	router  *replayRouter
	cfg     *replayConfig
	verify  *verifyLog
	div     *divergences
}

func newReplayRun(b *engine.ClusterBuilder, logs Logs, opts Options) (*replayRun, error) {
	div := &divergences{}
	r := &replayRun{
		engines: map[message.Address]Engine{},
		byTick:  map[message.Address]map[uint64][]replay.Record{},
		last:    map[message.Address]uint64{},
		cur:     map[message.Address]uint64{},
		needs:   map[message.Address]map[uint64][]replayNeed{},
		version: map[message.Address]uint64{},
		configs: map[uint64]*config.Snapshot{},
		opts:    opts,
		cfg:     &replayConfig{snap: config.Defaults()},
		div:     div,
	}
	reads := map[readKey]uint64{}
	need := map[message.Address]map[uint64]bool{}
	router := &replayRouter{
		Router: engine.NewRouter(), want: map[msgKey]message.Message{}, returned: map[msgKey]bool{},
		sent: map[msgKey]message.Message{}, refuse: map[msgKey]message.Reason{}, div: div,
	}
	queue := b.QueueSize()
	for address, records := range logs {
		perTick := map[uint64][]replay.Record{}
		counts := map[uint64]int{}
		for _, rec := range records {
			perTick[rec.Tick] = append(perTick[rec.Tick], rec)
			r.last[address] = max(r.last[address], rec.Tick)
			switch body := rec.Body.(type) {
			case replay.Inbound:
				router.want[keyOf(body.Message.Head())] = body.Message
				counts[rec.Tick]++
			case replay.Command:
				counts[rec.Tick]++
			case replay.Returned:
				router.returned[keyOf(body.Header)] = true
				counts[rec.Tick]++
			case replay.StableRead:
				reads[readKey{address, rec.Tick, body.Source}] = body.CopyTick
				if need[body.Source] == nil {
					need[body.Source] = map[uint64]bool{}
				}
				need[body.Source][body.CopyTick] = true
				r.last[body.Source] = max(r.last[body.Source], body.CopyTick)
				if body.CopyTick > 0 {
					r.needs[address] = nonNil(r.needs[address])
					r.needs[address][rec.Tick] = append(
						r.needs[address][rec.Tick], replayNeed{body.Source, body.CopyTick},
					)
				}
			}
		}
		for _, n := range counts {
			queue = max(queue, n+1) // the messages of a tick are all in the queue before it runs
		}
		r.byTick[address] = perTick
	}
	stable := &replayStable{
		worldID: b.WorldID(), reads: reads, need: need,
		world:      map[uint64]*worldstate.State{},
		zones:      map[state.ZoneID]map[uint64]*zonestate.State{},
		zonelayers: map[state.ZoneLayerID]map[uint64]*zonelayerstate.State{},
	}
	r.verify = &verifyLog{want: logs, pos: map[message.Address]int{}, div: div}
	r.router = router
	env := &engine.Env{
		Router: router, Cfg: r.cfg, ReplayLog: r.verify, Stable: stable,
		Graph: engine.NewWorldGraph(b.WorldID()), QueueSize: queue,
		WorldEmitter: engine.NopEmitter{}, ZoneEmitter: engine.NopEmitter{},
		ZoneLayerEmitter: engine.NopEmitter{},
	}
	var err error
	if r.cluster, err = b.BuildIn(env); err != nil {
		return nil, err
	}
	for _, e := range enginesOf(r.cluster) {
		r.engines[e.Address()] = e
		r.order = append(r.order, e.Address())
	}
	for address := range logs {
		if _, ok := r.engines[address]; !ok {
			return nil, fmt.Errorf(
				"the log of %v is not the log of an engine of the cluster", address,
			)
		}
	}
	return r, nil
}

func nonNil[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return map[K]V{}
	}
	return m
}

// ready reports whether the engine can run its tick: every copy that tick read exists.
func (r *replayRun) ready(e message.Address, tick uint64) bool {
	for _, n := range r.needs[e][tick] {
		if _, ok := r.engines[n.source]; ok && r.cur[n.source] < n.copyTick {
			return false
		}
	}
	return true
}

// play runs the engines until each has run to its last tick.
func (r *replayRun) play() error {
	for {
		progressed, done := false, true
		for _, e := range r.order {
			for r.cur[e] < r.last[e] && r.ready(e, r.cur[e]+1) {
				if err := r.stepEngine(e, r.cur[e]+1); err != nil {
					return err
				}
				progressed = true
			}
			if r.cur[e] < r.last[e] {
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
func (r *replayRun) snapshotOf(e message.Address, version uint64) *config.Snapshot {
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
func (r *replayRun) stepEngine(e message.Address, tick uint64) error {
	eng := r.engines[e]
	for _, rec := range r.byTick[e][tick] {
		switch body := rec.Body.(type) {
		case replay.ConfigRead:
			r.version[e] = body.Version
		case replay.Command:
			if err := eng.SendCommand(body.Command); err != nil {
				return fmt.Errorf("%v tick %d: %w", e, tick, err)
			}
		case replay.Inbound:
			if !r.router.inject(body.Message) {
				return fmt.Errorf("%v tick %d: a message could not be put in its queue", e, tick)
			}
		case replay.Returned:
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
	eng.Step()
	r.cur[e] = tick
	return nil
}

// finish reports what the log has that the replay did not make.
func (r *replayRun) finish() {
	for _, e := range r.order {
		records, i := r.verify.want[e], r.verify.pos[e]
		if i < len(records) {
			r.div.add(e, "tick %d: the log has a %T that the replay did not make",
				records[i].Tick, records[i].Body)
		}
	}
}
