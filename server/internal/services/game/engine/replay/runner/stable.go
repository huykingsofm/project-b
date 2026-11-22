package runner

import (
	"maps"

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
// the tick they were taken at, and gives each read the copy that the log says it found. A copy is
// let go as soon as the last read of it has been made, so what is kept follows the reads that are
// still to come and not the length of the log.
type replayStable struct {
	worldID    state.WorldID
	uses       map[copyKey]int    // how many reads of each copy are still to come
	reads      map[readKey]uint64 // the reads of the ticks that are loaded: which copy each finds
	world      map[uint64]*worldstate.State
	zones      map[state.ZoneID]map[uint64]*zonestate.State
	zonelayers map[state.ZoneLayerID]map[uint64]*zonelayerstate.State
}

func newReplayStable(worldID state.WorldID, uses map[copyKey]int) *replayStable {
	return &replayStable{
		worldID: worldID, uses: maps.Clone(uses), reads: map[readKey]uint64{},
		world:      map[uint64]*worldstate.State{},
		zones:      map[state.ZoneID]map[uint64]*zonestate.State{},
		zonelayers: map[state.ZoneLayerID]map[uint64]*zonelayerstate.State{},
	}
}

// expect says which copy a read of a tick that is about to run will find.
func (s *replayStable) expect(k readKey, copyTick uint64) { s.reads[k] = copyTick }

func (s *replayStable) kept(source message.Address, tick uint64) bool {
	return s.uses[copyKey{source, tick}] > 0
}

func (s *replayStable) PublishWorld(w *worldstate.State) {
	if s.kept(message.WorldAddress(s.worldID), w.Tick) {
		s.world[w.Tick] = w
	}
}

func (s *replayStable) PublishZone(z *zonestate.State) {
	if s.kept(message.ZoneAddress(z.ID), z.Tick) {
		if s.zones[z.ID] == nil {
			s.zones[z.ID] = map[uint64]*zonestate.State{}
		}
		s.zones[z.ID][z.Tick] = z
	}
}

func (s *replayStable) PublishZoneLayer(l *zonelayerstate.State) {
	if s.kept(message.ZoneLayerAddress(l.ID), l.Tick) {
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

// find returns which copy the read finds, and forgets that the read is to come.
func (s *replayStable) find(reader message.Address, tick uint64, source message.Address) uint64 {
	k := readKey{reader, tick, source}
	copyTick := s.reads[k]
	delete(s.reads, k)
	return copyTick
}

// done counts a read of a copy as made, and says whether it was the last.
func (s *replayStable) done(source message.Address, copyTick uint64) bool {
	k := copyKey{source, copyTick}
	if copyTick == 0 || s.uses[k] == 0 {
		return false
	}
	if s.uses[k]--; s.uses[k] > 0 {
		return false
	}
	delete(s.uses, k)
	return true
}

func (s *replayStable) CopyOfWorld(reader message.Address, tick uint64) *worldstate.State {
	source := message.WorldAddress(s.worldID)
	copyTick := s.find(reader, tick, source)
	w := s.world[copyTick]
	if s.done(source, copyTick) {
		delete(s.world, copyTick)
	}
	return w
}

func (s *replayStable) CopyOfZone(
	reader message.Address, tick uint64, zoneID state.ZoneID,
) *zonestate.State {
	source := message.ZoneAddress(zoneID)
	copyTick := s.find(reader, tick, source)
	z := s.zones[zoneID][copyTick]
	if s.done(source, copyTick) {
		delete(s.zones[zoneID], copyTick)
	}
	return z
}

func (s *replayStable) CopyOfZoneLayer(
	reader message.Address, tick uint64, zoneLayerID state.ZoneLayerID,
) *zonelayerstate.State {
	source := message.ZoneLayerAddress(zoneLayerID)
	copyTick := s.find(reader, tick, source)
	l := s.zonelayers[zoneLayerID][copyTick]
	if s.done(source, copyTick) {
		delete(s.zonelayers[zoneLayerID], copyTick)
	}
	return l
}

// held is how many copies are kept now.
func (s *replayStable) held() int {
	n := len(s.world)
	for _, m := range s.zones {
		n += len(m)
	}
	for _, m := range s.zonelayers {
		n += len(m)
	}
	return n
}
