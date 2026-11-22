package runner

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/engine"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
)

// readKey names a read of another engine: who read, at which tick, and whose copy.
type readKey struct {
	reader message.Address
	tick   uint64
	source message.Address
}

// replayNeed is a copy that a tick of an engine reads: whose, and the tick it was taken at.
type replayNeed struct {
	source   message.Address
	copyTick uint64
}

// replayStable is the registry of a replay: it keeps the copies that some read will ask for, by
// the tick they were taken at, and gives each read the copy that the log says it found.
type replayStable struct {
	worldID    state.WorldID
	reads      map[readKey]uint64
	need       map[message.Address]map[uint64]bool
	world      map[uint64]*worldstate.State
	zones      map[state.ZoneID]map[uint64]*zonestate.State
	zonelayers map[state.ZoneLayerID]map[uint64]*zonelayerstate.State
}

func (s *replayStable) PublishWorld(w *worldstate.State) {
	if s.need[message.WorldAddress(s.worldID)][w.Tick] {
		s.world[w.Tick] = w
	}
}

func (s *replayStable) PublishZone(z *zonestate.State) {
	if s.need[message.ZoneAddress(z.ID)][z.Tick] {
		if s.zones[z.ID] == nil {
			s.zones[z.ID] = map[uint64]*zonestate.State{}
		}
		s.zones[z.ID][z.Tick] = z
	}
}

func (s *replayStable) PublishZoneLayer(l *zonelayerstate.State) {
	if s.need[message.ZoneLayerAddress(l.ID)][l.Tick] {
		if s.zonelayers[l.ID] == nil {
			s.zonelayers[l.ID] = map[uint64]*zonelayerstate.State{}
		}
		s.zonelayers[l.ID][l.Tick] = l
	}
}

// RemoveZoneLayer keeps what was published: a read may still ask for it.
func (s *replayStable) RemoveZoneLayer(state.ZoneLayerID) {}

func (s *replayStable) View(
	reader message.Address, replayLog replay.Log,
) *engine.StableStateView {
	return engine.NewStableStateView(s, s.worldID, reader, replayLog)
}

func (s *replayStable) CopyOfWorld(reader message.Address, tick uint64) *worldstate.State {
	return s.world[s.reads[readKey{reader, tick, message.WorldAddress(s.worldID)}]]
}

func (s *replayStable) CopyOfZone(
	reader message.Address, tick uint64, zoneID state.ZoneID,
) *zonestate.State {
	return s.zones[zoneID][s.reads[readKey{reader, tick, message.ZoneAddress(zoneID)}]]
}

func (s *replayStable) CopyOfZoneLayer(
	reader message.Address, tick uint64, zoneLayerID state.ZoneLayerID,
) *zonelayerstate.State {
	copyTick := s.reads[readKey{reader, tick, message.ZoneLayerAddress(zoneLayerID)}]
	return s.zonelayers[zoneLayerID][copyTick]
}
