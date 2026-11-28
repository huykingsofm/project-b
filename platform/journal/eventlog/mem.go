package eventlog

import (
	"maps"
	"slices"
	"sort"
	"sync"

	"github.com/huykingsofm/project-b/platform/sim"
)

// MemLog keeps the records in memory.
type MemLog struct {
	mu      sync.Mutex
	Records []Record
}

// Append keeps a record.
func (j *MemLog) Append(a sim.StateID, t uint64, body Body) {
	j.add(Record{Engine: a, Tick: t, Body: body})
}

// EndTick does nothing: a MemLog has already kept every record.
func (j *MemLog) EndTick(sim.StateID, uint64) {}

// Safe says every tick is: a MemLog is for tests, which have no crash to lose a tick to.
func (j *MemLog) Safe(sim.StateID) uint64 { return ^uint64(0) }

// ByEngine groups the records that the log keeps by engine, each in the order it was kept. The
// slices are copies of what the log holds now.
func (j *MemLog) ByEngine() MemoryLogs {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := MemoryLogs{}
	for _, r := range j.Records {
		out[r.Engine] = append(out[r.Engine], r)
	}
	return out
}

// Engines returns the engines that have records, in the order of their IDs.
func (j *MemLog) Engines() []sim.StateID { return j.ByEngine().Engines() }

// Last returns the highest tick of a record of the engine.
func (j *MemLog) Last(engine sim.StateID) (uint64, error) { return j.ByEngine().Last(engine) }

// Open returns a source of the records of the engine from the tick from, so that a replay can read
// what a MemLog kept.
func (j *MemLog) Open(engine sim.StateID, from uint64) (Source, error) {
	return j.ByEngine().Open(engine, from)
}

func (j *MemLog) add(r Record) {
	j.mu.Lock()
	j.Records = append(j.Records, r)
	j.mu.Unlock()
}

// MemoryLogs holds the records of each engine in memory, each in the order of its ticks.
type MemoryLogs map[sim.StateID][]Record

// Engines returns the engines that have records, in the order of their IDs.
func (m MemoryLogs) Engines() []sim.StateID { return slices.Sorted(maps.Keys(m)) }

// Last returns the highest tick of a record of the engine.
func (m MemoryLogs) Last(engine sim.StateID) (uint64, error) {
	last := uint64(0)
	for _, r := range m[engine] {
		last = max(last, r.Tick)
	}
	return last, nil
}

// Open returns a source of the records of the engine from the tick from.
func (m MemoryLogs) Open(engine sim.StateID, from uint64) (Source, error) {
	records := m[engine]
	i := sort.Search(len(records), func(i int) bool { return records[i].Tick >= from })
	return &memorySource{records: records[i:]}, nil
}

// memorySource is the Source of the records of one engine that are held in memory.
type memorySource struct {
	records []Record
	next    int
	current Record
}

func (s *memorySource) Next() bool {
	if s.next >= len(s.records) {
		return false
	}
	s.current = s.records[s.next]
	s.next++
	return true
}

func (s *memorySource) Record() Record { return s.current }

func (s *memorySource) Err() error { return nil }

func (s *memorySource) Close() error { return nil }
