package engine

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/internal/shared/registry"
)

// StableRegistry is where the engines publish their states and where they read each other's: the
// StableStateRegistry of a cluster that runs, or the one a replay gives them, which serves the copy
// that the log says each read found.
type StableRegistry interface {
	PublishWorld(w *worldstate.State)
	PublishZone(z *zonestate.State)
	PublishZoneLayer(l *zonelayerstate.State)
	RemoveZoneLayer(zoneLayerID state.ZoneLayerID)
	View(reader message.Address, replayLog replay.Log) *StableStateView
}

// CopySource says which copy of an engine a StableStateView finds when its engine reads one at a
// tick. A registry that runs gives the latest; a replay gives the one that was read. A registry
// that is not the one of a cluster that runs makes its views with NewStableStateView.
type CopySource interface {
	CopyOfWorld(reader message.Address, tick uint64) *worldstate.State
	CopyOfZone(reader message.Address, tick uint64, zoneID state.ZoneID) *zonestate.State
	CopyOfZoneLayer(
		reader message.Address, tick uint64, zoneLayerID state.ZoneLayerID,
	) *zonelayerstate.State
}

var _ StableRegistry = (*StableStateRegistry)(nil)

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
func (s *StableStateRegistry) View(reader message.Address, replayLog replay.Log) *StableStateView {
	return NewStableStateView(s, s.worldID, reader, replayLog)
}

// CopyOfWorld, CopyOfZone and CopyOfZoneLayer return the latest published copy, or nil.
func (s *StableStateRegistry) CopyOfWorld(message.Address, uint64) *worldstate.State {
	return s.world.Get(s.worldID)
}
func (s *StableStateRegistry) CopyOfZone(
	_ message.Address, _ uint64, zoneID state.ZoneID,
) *zonestate.State {
	return s.zones.Get(zoneID)
}
func (s *StableStateRegistry) CopyOfZoneLayer(
	_ message.Address, _ uint64, zoneLayerID state.ZoneLayerID,
) *zonelayerstate.State {
	return s.zonelayers.Get(zoneLayerID)
}
