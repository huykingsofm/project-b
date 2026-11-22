package engine

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/internal/shared/registry"
)

// StableStateRegistry is the registry of the states that the engines published: the latest copy of
// the state of the world, of each zone and of each zonelayer.
//
// An engine publishes at the end of every tick a Clone of its state, and never changes that copy
// again. The other engines read the latest copy they find, with no lock and no wait, and keep it
// for as long as they need it; nothing has to be given back. What they read may be a few ticks old,
// and it is nil for an engine that published nothing yet or was removed. Only the engine that owns
// a piece of data knows exactly what it is.
type StableStateRegistry struct {
	world      registry.Registry[state.WorldID, worldstate.State]
	zones      registry.Registry[state.ZoneID, zonestate.State]
	zonelayers registry.Registry[state.ZoneLayerID, zonelayerstate.State]
	worldID    state.WorldID
}

// NewStableStateRegistry makes the registry of the world with the ID.
func NewStableStateRegistry(worldID state.WorldID) *StableStateRegistry {
	return &StableStateRegistry{worldID: worldID}
}

// PublishWorld, PublishZone and PublishZoneLayer make a copy the latest of its engine.
func (s *StableStateRegistry) PublishWorld(w *worldstate.State) { s.world.Publish(s.worldID, w) }
func (s *StableStateRegistry) PublishZone(z *zonestate.State)   { s.zones.Publish(z.ID, z) }
func (s *StableStateRegistry) PublishZoneLayer(l *zonelayerstate.State) {
	s.zonelayers.Publish(l.ID, l)
}

// RemoveZoneLayer forgets what a zonelayer published, as the zonelayer is removed.
func (s *StableStateRegistry) RemoveZoneLayer(zoneLayerID state.ZoneLayerID) {
	s.zonelayers.Remove(zoneLayerID)
}

// View returns what the engine at the address reads of the registry. The registry does not give its
// states out itself: an engine reads them through its own view, which keeps the first copy it reads
// of each scope in a tick (see StableStateView) and writes the read to the replay log.
func (s *StableStateRegistry) View(reader action.Address, replayLog replay.Log) *StableStateView {
	return &StableStateView{
		registry: s, replayLog: replayLog, me: reader,
		zones:      map[state.ZoneID]*zonestate.State{},
		zonelayers: map[state.ZoneLayerID]*zonelayerstate.State{},
	}
}

// latestWorld, latestZone and latestZoneLayer return the latest published copy, or nil.
func (s *StableStateRegistry) latestWorld() *worldstate.State { return s.world.Get(s.worldID) }
func (s *StableStateRegistry) latestZone(zoneID state.ZoneID) *zonestate.State {
	return s.zones.Get(zoneID)
}
func (s *StableStateRegistry) latestZoneLayer(zoneLayerID state.ZoneLayerID) *zonelayerstate.State {
	return s.zonelayers.Get(zoneLayerID)
}
