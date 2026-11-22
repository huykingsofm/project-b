package engine

import (
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/registry"
)

// StableStore is where the engines publish their states and where they read each other's: the
// StableStateStore of a cluster that runs, or the one a replay gives them, which serves the copy
// that the log says each read found.
type StableStore interface {
	// Publish makes a state the latest copy of its engine. The state must not be changed after.
	Publish(s sim.State)
	// Remove forgets what an engine published, as the engine is removed.
	Remove(id sim.StateID)
	// View returns what the engine with the ID reads of the store.
	View(reader sim.StateID, replayLog replay.Log) *StableView
}

// CopySource says which copy of an engine a StableView finds when its engine reads one at a tick. A
// store that runs gives the latest; a replay gives the one that was read. It returns nil for an
// engine that published none.
type CopySource interface {
	Copy(reader sim.StateID, tick uint64, id sim.StateID) sim.State
}

var _ StableStore = (*StableStateStore)(nil)

// StableStateStore is the store of the states that the engines published: the latest copy of the
// state of each engine, whatever its type.
//
// An engine publishes at the end of every tick a Snapshot of its state, and never changes that copy
// again. The other engines read the latest copy they find, with no lock and no wait, and keep it
// for as long as they need it; nothing has to be given back. What they read may be a few ticks old,
// and it is not found for an engine that published nothing yet or was removed. Only the engine that
// owns a piece of data knows exactly what it is.
type StableStateStore struct {
	states registry.Registry[sim.StateID, sim.State]
}

// NewStableStateStore makes an empty store.
func NewStableStateStore() *StableStateStore { return &StableStateStore{} }

// Publish makes a copy the latest of its engine.
func (s *StableStateStore) Publish(st sim.State) { s.states.Publish(st.StateID(), st) }

// Remove forgets what an engine published.
func (s *StableStateStore) Remove(id sim.StateID) { s.states.Remove(id) }

// View returns what the engine with the ID reads of the store. The store does not give its states
// out itself: an engine reads them through its own view, which keeps the first copy it reads of
// each ID in a tick (see StableView) and writes the read to the replay log.
func (s *StableStateStore) View(reader sim.StateID, replayLog replay.Log) *StableView {
	return NewStableView(s, reader, replayLog)
}

// Copy returns the latest published copy, or nil.
func (s *StableStateStore) Copy(_ sim.StateID, _ uint64, id sim.StateID) sim.State {
	st, _ := s.states.Get(id)
	return st
}

// StableView is what one engine reads of the StableStore in a tick. The first time the engine reads
// a state in the tick it keeps the copy it got, and every later read of that state in the tick
// returns the same copy, so what an engine knows of the others does not change while its plan runs
// (the store itself gives the latest copy at the moment of the read, and the other engines publish
// at any time; a replay gives the copy that the log says was read). The first read of each state in
// a tick is written to the replay log, with the tick of the copy, so that a replay can give the
// engine the same one. Only the goroutine of its engine uses it, so it takes no lock.
type StableView struct {
	source    CopySource
	replayLog replay.Log
	me        sim.StateID
	tick      uint64
	states    map[sim.StateID]sim.State
}

var _ sim.StableReader = (*StableView)(nil)

// NewStableView makes the view of the engine with the ID, which finds the copies that the source
// gives, and writes each first read of a tick to the replay log.
func NewStableView(source CopySource, reader sim.StateID, replayLog replay.Log) *StableView {
	return &StableView{
		source: source, replayLog: replayLog, me: reader, states: map[sim.StateID]sim.State{},
	}
}

// begin forgets what was kept: a new tick reads again.
func (v *StableView) begin(tick uint64) {
	v.tick = tick
	clear(v.states)
}

// Read returns the copy of the state that this tick read first, or sim.ErrStateNotFound.
func (v *StableView) Read(id sim.StateID) (sim.State, error) {
	st, ok := v.states[id]
	if !ok {
		st = v.source.Copy(v.me, v.tick, id)
		v.states[id] = st
		var copyTick uint64
		if st != nil {
			copyTick = st.StateTick()
		}
		v.replayLog.Append(v.me, v.tick, replay.StableRead{Source: id, CopyTick: copyTick})
	}
	if st == nil {
		return nil, sim.ErrStateNotFound
	}
	return st, nil
}
