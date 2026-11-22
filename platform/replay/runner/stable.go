package runner

import (
	"maps"

	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
)

// readKey names a read of another engine: who read, at which tick, and whose copy.
type readKey struct {
	reader sim.StateID
	tick   uint64
	source sim.StateID
}

// replayNeed is a copy that a tick of an engine reads: whose, and the tick it was taken at.
type replayNeed struct {
	source   sim.StateID
	copyTick uint64
}

// replayStable is the store of a replay: it keeps the copies that some read will ask for, by
// the tick they were taken at, and gives each read the copy that the log says it found. A copy is
// let go as soon as the last read of it has been made, so what is kept follows the reads that are
// still to come and not the length of the log.
type replayStable struct {
	uses   map[copyKey]int    // how many reads of each copy are still to come
	reads  map[readKey]uint64 // the reads of the ticks that are loaded: which copy each finds
	states map[sim.StateID]map[uint64]sim.State
}

var _ engine.StableStore = (*replayStable)(nil)

func newReplayStable(uses map[copyKey]int) *replayStable {
	return &replayStable{
		uses: maps.Clone(uses), reads: map[readKey]uint64{},
		states: map[sim.StateID]map[uint64]sim.State{},
	}
}

// expect says which copy a read of a tick that is about to run will find.
func (s *replayStable) expect(k readKey, copyTick uint64) { s.reads[k] = copyTick }

func (s *replayStable) kept(source sim.StateID, tick uint64) bool {
	return s.uses[copyKey{source, tick}] > 0
}

// Publish keeps a copy that some read will ask for.
func (s *replayStable) Publish(st sim.State) {
	id, tick := st.StateID(), st.StateTick()
	if s.kept(id, tick) {
		if s.states[id] == nil {
			s.states[id] = map[uint64]sim.State{}
		}
		s.states[id][tick] = st
	}
}

// Remove keeps what was published: a read may still ask for it.
func (s *replayStable) Remove(sim.StateID) {}

func (s *replayStable) View(reader sim.StateID, replayLog replay.Log) *engine.StableView {
	return engine.NewStableView(s, reader, replayLog)
}

// find returns which copy the read finds, and forgets that the read is to come.
func (s *replayStable) find(reader sim.StateID, tick uint64, source sim.StateID) uint64 {
	k := readKey{reader, tick, source}
	copyTick := s.reads[k]
	delete(s.reads, k)
	return copyTick
}

// done counts a read of a copy as made, and says whether it was the last.
func (s *replayStable) done(source sim.StateID, copyTick uint64) bool {
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

// Copy gives the copy that the log says the read found.
func (s *replayStable) Copy(reader sim.StateID, tick uint64, source sim.StateID) sim.State {
	copyTick := s.find(reader, tick, source)
	st := s.states[source][copyTick]
	if s.done(source, copyTick) {
		delete(s.states[source], copyTick)
	}
	return st
}

// held is how many copies are kept now.
func (s *replayStable) held() int {
	n := 0
	for _, m := range s.states {
		n += len(m)
	}
	return n
}
