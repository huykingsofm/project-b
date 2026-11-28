package replay

import (
	"maps"

	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/runtime/router"
	"github.com/huykingsofm/project-b/platform/runtime/stable"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/stablemap"
)

// What follows are the adapters that stand in for the parts of the cluster that a replay does not
// have: the router (which does not deliver what the engines send but compares it), the published
// states (which give each read the copy the log says it found), and the event
// log (which compares what the engine records with what the log holds).

// messageKey names a message: who sent it and which message of the sender it was.
type messageKey struct {
	from sim.StateID
	seq  uint64
}

func keyOf(h sim.Header) messageKey { return messageKey{h.From, h.Seq} }

// replayRouter is the router of a replay. It carries nothing to the engines by itself: the replay
// puts in the channel of an engine the messages that the log of that engine says it took. What an
// engine sends is compared with what the receivers recorded and kept when the log says it came
// back, so that it can be given back at the tick the log says.
type replayRouter struct {
	*router.Router

	// what the log says of the messages: the hash of each as its receiver took it, which came back
	// undelivered, and what the router answers at once, at the send
	inboundMessageHash map[messageKey]uint64
	msgsToBeReturned   map[messageKey]bool
	refuse             map[messageKey]sim.DeliveryFailure

	// the messages that were sent, and the divergences found
	sent map[messageKey]sim.Message
	div  *divergences
}

func (r *replayRouter) Deliver(m sim.Message) (sim.DeliveryFailure, bool) {
	h := m.Head()
	key := keyOf(h)
	if inbound, ok := r.inboundMessageHash[key]; ok && inbound != eventlog.HashMessage(m) {
		r.div.add(h.From, "the message %d it sends is not the one that %v recorded as taken", h.Seq,
			m.Dest())
	}
	if r.msgsToBeReturned[key] {
		r.sent[key] = m
	}
	if reason, ok := r.refuse[key]; ok {
		delete(r.refuse, key)
		return reason, false
	}
	return 0, true
}

// inject puts a message in the channel of the engine it is for.
func (r *replayRouter) inject(m sim.Message) bool {
	_, ok := r.Router.Deliver(m)
	return ok
}

// readKey names a read of another engine: who read, at which tick, and whose copy.
type readKey struct {
	reader sim.StateID
	tick   uint64
	source sim.StateID
}

// replayStable is the store of a replay: it keeps the copies that some read will ask for, by
// the tick they were taken at, and gives each read the copy that the log says it found. A copy is
// let go as soon as the last read of it has been made, so what is kept follows the reads that are
// still to come and not the length of the log.
type replayStable struct {
	copyReads map[copyKey]int    // how many reads of each copy are still to come
	reads     map[readKey]uint64 // the reads of the ticks that are loaded: which copy each finds
	states    map[sim.StateID]map[uint64]sim.State
}

func newReplayStable(copyReads map[copyKey]int) *replayStable {
	return &replayStable{
		copyReads: maps.Clone(copyReads), reads: map[readKey]uint64{},
		states: map[sim.StateID]map[uint64]sim.State{},
	}
}

// Publish keeps a copy that some read will ask for.
func (s *replayStable) Publish(id sim.StateID, st sim.State) {
	tick := st.Tick()
	if s.kept(id, tick) {
		if s.states[id] == nil {
			s.states[id] = map[uint64]sim.State{}
		}
		s.states[id][tick] = st
	}
}

// Remove keeps what was published: a read may still ask for it.
func (s *replayStable) Remove(sim.StateID) {}

func (s *replayStable) View(reader sim.StateID, eventLog eventlog.Log) *stable.View {
	return stable.NewView(s, reader, eventLog)
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

// expect says which copy a read of a tick that is about to run will find.
func (s *replayStable) expect(k readKey, copyTick uint64) { s.reads[k] = copyTick }

func (s *replayStable) kept(source sim.StateID, tick uint64) bool {
	return s.copyReads[copyKey{source, tick}] > 0
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
	if copyTick == 0 || s.copyReads[k] == 0 {
		return false
	}
	if s.copyReads[k]--; s.copyReads[k] > 0 {
		return false
	}
	delete(s.copyReads, k)
	return true
}

var _ stable.Store = (*replayStable)(nil)

// verifyLog is the event log of a replay: it checks that the engine records, in the same order,
// exactly what it recorded the first time. A hash is worked out and compared.
type verifyLog struct {
	engines *stablemap.Map[sim.StateID, *engineRun]
	div     *divergences
}

// Append checks what the engine records against the next record of its log.
func (v *verifyLog) Append(e sim.StateID, tick uint64, body eventlog.Body) {
	v.expect(e, eventlog.Record{Tick: tick, Body: body})
}

// EndTick checks that the engine made every record that the log has for the tick.
// Safe says every tick is: a replay compares records and keeps none.
func (v *verifyLog) Safe(sim.StateID) uint64 { return ^uint64(0) }

func (v *verifyLog) EndTick(e sim.StateID, _ uint64) {
	er, ok := v.engines.Get(e)
	if !ok || !er.loaded {
		return
	}
	if er.recordsMade < len(er.tickRecords) {
		missing := er.tickRecords[er.recordsMade]
		v.div.add(e, "tick %d: the log has a %T that the replay did not make",
			missing.Tick, missing.Body)
		er.recordsMade = len(er.tickRecords)
	}
}

func (v *verifyLog) expect(engine sim.StateID, got eventlog.Record) {
	var records []eventlog.Record
	i := 0
	if er, ok := v.engines.Get(engine); ok && er.loaded {
		records = er.tickRecords
		i = er.recordsMade
		er.recordsMade++
	}
	if i >= len(records) {
		v.div.add(engine,
			"tick %d: the replay recorded a %T that the log does not have", got.Tick, got.Body)
		return
	}
	if want := records[i]; want.Tick != got.Tick || !want.Body.Same(got.Body) {
		v.div.add(engine,
			"tick %d: the log has %T (%+v), the replay made %T at tick %d (%+v)",
			want.Tick, want.Body, want.Body, got.Body, got.Tick, got.Body)
	}
}
