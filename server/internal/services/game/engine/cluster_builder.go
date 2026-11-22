package engine

import (
	"errors"
	"fmt"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"

	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap"
)

// WorldParams are the parameters of the world to build.
type WorldParams struct {
	ID state.WorldID
}

// ZoneParams are the parameters of a zone to build: its map, and which zones its exits lead to.
type ZoneParams struct {
	ID       state.ZoneID
	Tiles    *tilemap.TileMap
	Adjacent []state.ZoneID
}

// ZoneLayerParams are the parameters of the zonelayers of one zone: how many it starts with, how
// many characters each takes, and every how many ticks they heal their characters (0 for never).
type ZoneLayerParams struct {
	ZoneID     state.ZoneID
	Count      int
	Capacity   int32
	RegenEvery uint64
}

// ClusterBuilder collects what a cluster is made of and builds it. NewBuilder gives one with a
// default for everything that has one; each With method sets one thing and returns the builder, so
// calls chain, and Build makes the cluster.
type ClusterBuilder struct {
	cfg              *config.Store
	seed             uint64
	queueSize        int
	replayLog        replay.Log
	worldEmitter     WorldEmitter
	zoneEmitter      ZoneEmitter
	zoneLayerEmitter ZoneLayerEmitter

	world      *WorldParams
	zones      []ZoneParams
	zoneLayers []ZoneLayerParams
}

// NewBuilder returns a builder with the defaults: a configuration store with every default value,
// seed 0, a queue of 256 for each engine, a replay log that records nothing, and emitters that
// discard.
func NewBuilder() *ClusterBuilder {
	return &ClusterBuilder{
		cfg:              config.NewStore(),
		queueSize:        256,
		replayLog:        replay.NopLog{},
		worldEmitter:     NopEmitter{},
		zoneEmitter:      NopEmitter{},
		zoneLayerEmitter: NopEmitter{},
	}
}

// WithConfig sets the configuration store that the engines read. A nil store keeps the default.
func (b *ClusterBuilder) WithConfig(cfg *config.Store) *ClusterBuilder {
	if cfg != nil {
		b.cfg = cfg
	}
	return b
}

// WithSeed sets the seed that the random numbers of the engines come from.
func (b *ClusterBuilder) WithSeed(seed uint64) *ClusterBuilder {
	b.seed = seed
	return b
}

// WithQueueSize sets how many commands and messages the queue of an engine holds.
func (b *ClusterBuilder) WithQueueSize(size int) *ClusterBuilder {
	b.queueSize = size
	return b
}

// WithReplayLog sets where the engines write what a replay needs. A nil log keeps the default.
func (b *ClusterBuilder) WithReplayLog(log replay.Log) *ClusterBuilder {
	if log != nil {
		b.replayLog = log
	}
	return b
}

// WithWorldEmitter sets where the world engine gives its changes. A nil emitter keeps the default.
func (b *ClusterBuilder) WithWorldEmitter(e WorldEmitter) *ClusterBuilder {
	if e != nil {
		b.worldEmitter = e
	}
	return b
}

// WithZoneEmitter sets where the zone engines give their changes. A nil emitter keeps the default.
func (b *ClusterBuilder) WithZoneEmitter(e ZoneEmitter) *ClusterBuilder {
	if e != nil {
		b.zoneEmitter = e
	}
	return b
}

// WithZoneLayerEmitter sets where the zonelayer engines give their changes. A nil emitter keeps the
// default.
func (b *ClusterBuilder) WithZoneLayerEmitter(e ZoneLayerEmitter) *ClusterBuilder {
	if e != nil {
		b.zoneLayerEmitter = e
	}
	return b
}

// WithWorld sets the parameters of the world.
func (b *ClusterBuilder) WithWorld(p WorldParams) *ClusterBuilder {
	b.world = &p
	return b
}

// WithZone adds a zone. The zones are built in the order they were added.
func (b *ClusterBuilder) WithZone(p ZoneParams) *ClusterBuilder {
	b.zones = append(b.zones, p)
	return b
}

// WithZoneLayers sets the zonelayers that a zone starts with. A zone that has none has no
// zonelayer.
func (b *ClusterBuilder) WithZoneLayers(p ZoneLayerParams) *ClusterBuilder {
	b.zoneLayers = append(b.zoneLayers, p)
	return b
}

// check says what is wrong with what the builder holds, if anything.
func (b *ClusterBuilder) check() error {
	if b.world == nil {
		return errors.New("engine: no world: call WithWorld")
	}
	if b.queueSize <= 0 {
		return fmt.Errorf("engine: the queue size is %d, want more than 0", b.queueSize)
	}
	zones := map[state.ZoneID]bool{}
	for _, z := range b.zones {
		if z.Tiles == nil {
			return fmt.Errorf("engine: zone %d has no map", z.ID)
		}
		if zones[z.ID] {
			return fmt.Errorf("engine: zone %d is added twice", z.ID)
		}
		zones[z.ID] = true
	}
	withLayers := map[state.ZoneID]bool{}
	for _, l := range b.zoneLayers {
		if !zones[l.ZoneID] {
			return fmt.Errorf("engine: zonelayers for zone %d, which was not added", l.ZoneID)
		}
		if withLayers[l.ZoneID] {
			return fmt.Errorf("engine: zonelayers of zone %d are set twice", l.ZoneID)
		}
		withLayers[l.ZoneID] = true
		if l.Count < 0 || (l.Count > 0 && l.Capacity <= 0) {
			return fmt.Errorf(
				"engine: zone %d has %d zonelayers of capacity %d", l.ZoneID, l.Count, l.Capacity,
			)
		}
	}
	return nil
}

// Build makes the engines of the world and connects them. Nothing runs yet: give the ticks with
// Step, or start them with Run. It returns an error when what the builder holds cannot make a
// cluster.
func (b *ClusterBuilder) Build() (*Cluster, error) {
	if err := b.check(); err != nil {
		return nil, err
	}
	env := &Env{
		Router: NewRouter(), Cfg: b.cfg, ReplayLog: b.replayLog, Stable: NewStableStateRegistry(
			b.world.ID,
		),
		Graph: NewWorldGraph(b.world.ID), QueueSize: b.queueSize,
		WorldEmitter: b.worldEmitter, ZoneEmitter: b.zoneEmitter,
		ZoneLayerEmitter: b.zoneLayerEmitter,
	}
	layers := map[state.ZoneID]ZoneLayerParams{}
	for _, l := range b.zoneLayers {
		layers[l.ZoneID] = l
	}

	c := &Cluster{Env: env}
	c.World = NewWorldEngine(worldstate.NewState(b.world.ID), env, b.seed)

	var nextZoneLayerID state.ZoneLayerID
	for _, zp := range b.zones {
		env.Graph.AddZone(zp.ID, zp.Adjacent...)
		zst := zonestate.NewState(zp.ID, zp.Tiles)
		lp := layers[zp.ID]
		var zonelayers []*ZoneLayerEngine
		for i := 0; i < lp.Count; i++ {
			nextZoneLayerID++
			zst.AddZoneLayer(nextZoneLayerID, lp.Capacity)
			env.Graph.AddZoneLayer(zp.ID, nextZoneLayerID)
			le := NewZoneLayerEngine(zonelayerstate.NewState(nextZoneLayerID, zp.ID), env, b.seed)
			if lp.RegenEvery > 0 {
				le.AddCron(lp.RegenEvery, &action.ZoneLayerCharacterRegenAction{})
			}
			zonelayers = append(zonelayers, le)
		}
		c.zones = append(c.zones, NewZoneEngine(zst, env, b.seed))
		c.zonelayers = append(c.zonelayers, zonelayers...)
	}
	return c, nil
}
