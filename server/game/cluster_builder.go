package game

import (
	"errors"
	"fmt"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
)

// WorldParams are the parameters of the world to build. The ID of the world is made with
// sim.NewStateID(state.WorldType, n) and that of a zone with state.ZoneType; the builder gives the
// zonelayers the numbers 1, 2, 3 among the states of their type.
type WorldParams struct {
	ID sim.StateID
}

// ZoneParams are the parameters of a zone to build: its ID and its map, whose exits say which zones
// they lead to.
type ZoneParams struct {
	ID    sim.StateID
	Tiles *tilemap.TileMap
}

// ZoneLayerParams are the parameters of the zonelayers of one zone: how many it starts with, how
// many characters each takes, and every how many ticks they heal their characters (0 for never).
type ZoneLayerParams struct {
	ZoneID     sim.StateID
	Count      int
	Capacity   int32
	RegenEvery uint64
}

// ClusterBuilder collects what a cluster is made of and builds it. NewBuilder gives one with a
// default for everything that has one; each With method sets one thing and returns the builder, so
// calls chain, and Build makes the cluster.
type ClusterBuilder struct {
	seed      uint64
	queueSize int
	eventLog  eventlog.Log
	emitter   engine.Emitter
	// store is the configuration of the world outside the game, which the cluster sends to the
	// configuration engine
	store *config.Store
	// checkpoints writes the checkpoints the engines take; nil: they take none
	checkpoints engine.CheckpointWriter

	world      *WorldParams
	zones      []ZoneParams
	zoneLayers []ZoneLayerParams
}

// NewBuilder returns a builder with the defaults:
// seed 0, a queue of 256 for each engine, a event log that records nothing, an emitter that
// discards, and no writer of checkpoints, so the engines take none.
func NewBuilder() *ClusterBuilder {
	return &ClusterBuilder{
		queueSize: 256,
		eventLog:  eventlog.NopLog{},
		emitter:   engine.NopEmitter{},
		store:     config.NewStore(),
	}
}

// WithSeed sets the seed that the random numbers of the engines come from.
func (b *ClusterBuilder) WithSeed(seed uint64) *ClusterBuilder {
	b.seed = seed
	return b
}

// Seed is the seed that the random numbers of the engines come from.
func (b *ClusterBuilder) Seed() uint64 { return b.seed }

// WithQueueSize sets how many commands and messages the queue of an engine holds.
func (b *ClusterBuilder) WithQueueSize(size int) *ClusterBuilder {
	b.queueSize = size
	return b
}

// WithConfigStore sets the configuration of the world outside the game, which the cluster sends to
// its configuration engine each time it changes. It is for what is made before the cluster and
// needs it too (see replay.NewLineCacheWriter). A nil one keeps the default, which no one else
// holds.
func (b *ClusterBuilder) WithConfigStore(store *config.Store) *ClusterBuilder {
	if store != nil {
		b.store = store
	}
	return b
}

// WithEventLog sets where the engines write what a replay needs. A nil log keeps the default.
func (b *ClusterBuilder) WithEventLog(log eventlog.Log) *ClusterBuilder {
	if log != nil {
		b.eventLog = log
	}
	return b
}

// WithCheckpointWriter sets what writes the checkpoints that the engines take at the end of a tick
// (see engine.CheckpointWriter). With none, which is the default, they take none.
func (b *ClusterBuilder) WithCheckpointWriter(w engine.CheckpointWriter) *ClusterBuilder {
	b.checkpoints = w
	return b
}

// WithEmitter sets where the engines give their changes. A nil emitter keeps the default.
func (b *ClusterBuilder) WithEmitter(e engine.Emitter) *ClusterBuilder {
	if e != nil {
		b.emitter = e
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

// QueueSize is how many commands and messages the queue of an engine holds.
func (b *ClusterBuilder) QueueSize() int { return b.queueSize }

// tilesOf is the map of the zone with the ID, or nil.
func (b *ClusterBuilder) tilesOf(zoneID sim.StateID) *tilemap.TileMap {
	for _, z := range b.zones {
		if z.ID == zoneID {
			return z.Tiles
		}
	}
	return nil
}

// Codecs makes the codecs that read back what the engines of the cluster write. The builder must
// hold the zones, because the map of a zone is not written with its state. It returns nil when the
// codecs cannot be made, which Build reports as an error.
func (b *ClusterBuilder) Codecs() *sim.Codecs {
	c, err := NewCodecs(b.tilesOf)
	if err != nil {
		return nil
	}
	return c
}

// check says what is wrong with what the builder holds, if anything.
func (b *ClusterBuilder) check() error {
	if b.world == nil {
		return errors.New("game: no world: call WithWorld")
	}
	if b.queueSize <= 0 {
		return fmt.Errorf("game: the queue size is %d, want more than 0", b.queueSize)
	}
	if b.world.ID.Type() != state.WorldType {
		return fmt.Errorf("game: the ID %d of the world is not of the type of a world", b.world.ID)
	}
	zones := map[sim.StateID]bool{}
	for _, z := range b.zones {
		if z.Tiles == nil {
			return fmt.Errorf("game: zone %d has no map", z.ID)
		}
		if zones[z.ID] {
			return fmt.Errorf("game: zone %d is added twice", z.ID)
		}
		if z.ID.Type() != state.ZoneType {
			return fmt.Errorf("game: the ID %d of a zone is not of the type of a zone", z.ID)
		}
		zones[z.ID] = true
	}
	withLayers := map[sim.StateID]bool{}
	for _, l := range b.zoneLayers {
		if !zones[l.ZoneID] {
			return fmt.Errorf("game: zonelayers for zone %d, which was not added", l.ZoneID)
		}
		if withLayers[l.ZoneID] {
			return fmt.Errorf("game: zonelayers of zone %d are set twice", l.ZoneID)
		}
		withLayers[l.ZoneID] = true
		if l.Count < 0 || (l.Count > 0 && l.Capacity <= 0) {
			return fmt.Errorf(
				"game: zone %d has %d zonelayers of capacity %d", l.ZoneID, l.Count, l.Capacity,
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
	codecs, err := NewCodecs(b.tilesOf)
	if err != nil {
		return nil, err
	}
	env := &engine.Env{
		Router: engine.NewRouter(), EventLog: b.eventLog,
		Stable: engine.NewStableStateStore(), Codecs: codecs, QueueSize: b.queueSize,
		Emitter: b.emitter, Acks: engine.NewAckBoard(), Checkpoints: b.checkpoints,
	}
	return b.assemble(env, engine.NoCheckpoints{})
}

// BuildIn makes the engines of the world and connects them in the environment given instead of one
// of its own: a replay gives the engines its own router, published states, configuration, replay
// log and emitter. An engine for which checkpoints has a checkpoint is made from it, at its tick;
// the others are made new. It returns an error when what the builder holds cannot make a cluster,
// or a checkpoint cannot make its engine.
func (b *ClusterBuilder) BuildIn(
	env *engine.Env, checkpoints engine.CheckpointSource,
) (*engine.Cluster, error) {
	if err := b.check(); err != nil {
		return nil, err
	}
	c, err := b.assemble(env, checkpoints)
	if err != nil {
		return nil, err
	}
	return c.Cluster, nil
}

// assemble makes the engines of the world in the environment given and connects them: the same
// engines whether the environment is the one of a cluster that runs or the one of a replay. The
// zonelayers are added to the cluster first, then the zones, then the world, which is the order
// that Step runs them in. An engine whose checkpoint the source has is made from it, whatever the
// state that was just made says: the states made here give the engines their IDs, and nothing else.
func (b *ClusterBuilder) assemble(
	env *engine.Env, checkpoints engine.CheckpointSource,
) (*Cluster, error) {
	layers := map[sim.StateID]ZoneLayerParams{}
	for _, l := range b.zoneLayers {
		layers[l.ZoneID] = l
	}
	worldState := worldstate.NewState(b.world.ID)
	var zoneEngines []*engine.Engine[*zonestate.State]
	var layerEngines []*engine.Engine[*zonelayerstate.State]
	for _, zp := range b.zones {
		worldState.AddZone(zp.ID)
		zoneState := zonestate.NewState(zp.ID, b.world.ID, zp.Tiles)
		lp := layers[zp.ID]
		for range lp.Count {
			layerID := worldState.AddZoneLayer()
			zoneState.AddZoneLayer(layerID, lp.Capacity)
			layerState := zonelayerstate.NewState(layerID, zp.ID, b.world.ID)
			le, err := engine.NewOrRestore(layerState, env, b.seed, checkpoints)
			if err != nil {
				return nil, err
			}
			if err := EnsureRegen(le, lp.RegenEvery); err != nil {
				return nil, err
			}
			layerEngines = append(layerEngines, le)
		}
		ze, err := engine.NewOrRestore(zoneState, env, b.seed, checkpoints)
		if err != nil {
			return nil, err
		}
		zoneEngines = append(zoneEngines, ze)
	}
	world, err := engine.NewOrRestore(worldState, env, b.seed, checkpoints)
	if err != nil {
		return nil, err
	}

	base, err := engine.NewCluster(env, b.seed, checkpoints, b.store)
	if err != nil {
		return nil, err
	}
	c := &Cluster{Cluster: base, World: world}
	for _, l := range layerEngines {
		c.Add(l)
	}
	for _, z := range zoneEngines {
		c.Add(z)
	}
	c.Add(c.World)
	return c, nil
}
