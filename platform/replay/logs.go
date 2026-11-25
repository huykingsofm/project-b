package replay

import (
	"fmt"
	"maps"
	"slices"
	"sort"

	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// Logs is where the records of a run come from: a source for each engine that recorded something,
// which can be opened as often as needed. A replay reads each once to index it and once to run it.
// A source gives the records of the tick from and of the ticks after it, as an engine that starts
// from a checkpoint needs no more.
type Logs interface {
	Engines() []sim.StateID
	// Last is the last tick of the log of the engine, 0 when it holds none.
	Last(engine sim.StateID) (uint64, error)
	Open(engine sim.StateID, from uint64) (eventlog.Source, error)
}

// MemoryLogs holds the records of each engine in memory.
type MemoryLogs map[sim.StateID][]eventlog.Record

// Engines returns the engines that have records, in the order of their addresses.
func (m MemoryLogs) Engines() []sim.StateID { return sortedIDs(maps.Keys(m)) }

// Last returns the highest tick of a record of the engine.
func (m MemoryLogs) Last(engine sim.StateID) (uint64, error) {
	last := uint64(0)
	for _, r := range m[engine] {
		last = max(last, r.Tick)
	}
	return last, nil
}

// Open returns a source of the records of the engine from the tick from.
func (m MemoryLogs) Open(engine sim.StateID, from uint64) (eventlog.Source, error) {
	records := m[engine]
	i := sort.Search(len(records), func(i int) bool { return records[i].Tick >= from })
	return &memorySource{records: records[i:]}, nil
}

type memorySource struct {
	records []eventlog.Record
	next    int
	current eventlog.Record
}

func (s *memorySource) Next() bool {
	if s.next >= len(s.records) {
		return false
	}
	s.current = s.records[s.next]
	s.next++
	return true
}

func (s *memorySource) Record() eventlog.Record { return s.current }
func (s *memorySource) Err() error              { return nil }
func (s *memorySource) Close() error            { return nil }

// fileLogs reads the files that a eventlog.FileLog wrote.
type fileLogs struct {
	dir     string
	codecs  *sim.Codecs
	engines []sim.StateID
}

// FileLogs reads the files that a eventlog.FileLog wrote under dir, which are opened one at a time
// and read as the run goes on.
func FileLogs(dir string, codecs *sim.Codecs) (Logs, error) {
	engines, err := eventlog.Engines(dir)
	if err != nil {
		return nil, err
	}
	return fileLogs{dir: dir, codecs: codecs, engines: sortedIDs(slices.Values(engines))}, nil
}

func (f fileLogs) Engines() []sim.StateID { return f.engines }

func (f fileLogs) Last(engine sim.StateID) (uint64, error) {
	return segmentlog.LastFrameID(eventlog.EngineDir(f.dir, engine))
}

func (f fileLogs) Open(engine sim.StateID, from uint64) (eventlog.Source, error) {
	r, err := eventlog.ReadFileLog(f.dir, engine, from, f.codecs)
	if err != nil {
		return nil, err
	}
	return r, nil // a *eventlog.FileLogReader is a Source and a Seeded
}

func sortedIDs(ids func(yield func(sim.StateID) bool)) []sim.StateID {
	out := slices.Collect(ids)
	slices.Sort(out)
	return out
}

// cursor reads the records of one engine a tick at a time, in the order of the ticks, and holds
// the first record of the next tick while it waits for it.
type cursor struct {
	src   eventlog.Source
	ahead eventlog.Record
	have  bool
}

func newCursor(src eventlog.Source) *cursor {
	c := &cursor{src: src}
	c.advance()
	return c
}

func (c *cursor) advance() { c.have = c.src.Next(); c.ahead = c.src.Record() }

// take returns the records of the tick, which must be asked for in rising order, one after another.
// A record of an earlier tick than the one asked for means the log is not in the order of its
// ticks.
func (c *cursor) take(tick uint64) ([]eventlog.Record, error) {
	var out []eventlog.Record
	for c.have && c.ahead.Tick <= tick {
		if c.ahead.Tick < tick {
			return nil, fmt.Errorf(
				"a record of the tick %d comes after the tick %d", c.ahead.Tick, tick,
			)
		}
		out = append(out, c.ahead)
		c.advance()
	}
	if !c.have {
		return out, c.src.Err()
	}
	return out, nil
}

func (c *cursor) close() error { return c.src.Close() }
