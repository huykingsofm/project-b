package runner

import (
	"maps"
	"slices"

	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
)

// Source gives the records of one engine in the order they were recorded, one at a time: the log
// of a run that lasted hours does not fit in memory.
type Source interface {
	Next() bool
	Record() replay.Record
	Err() error
	Close() error
}

// Logs is where the records of a run come from: a source for each engine that recorded something,
// which can be opened as often as needed. The runner reads each once to index it and once to run
// it.
type Logs interface {
	Engines() []sim.StateID
	Open(engine sim.StateID) (Source, error)
}

// Seeded is a Source that knows the seed of the run it was recorded in, as the files of a
// replay.FileLog do. The runner refuses a log whose seed is not the one of the builder.
type Seeded interface {
	Seed() uint64
}

// MemoryLogs holds the records of each engine in memory.
type MemoryLogs map[sim.StateID][]replay.Record

// Engines returns the engines that have records, in the order of their addresses.
func (m MemoryLogs) Engines() []sim.StateID { return sortedIDs(maps.Keys(m)) }

// Open returns a source of the records of the engine.
func (m MemoryLogs) Open(engine sim.StateID) (Source, error) {
	return &memorySource{records: m[engine]}, nil
}

type memorySource struct {
	records []replay.Record
	next    int
	current replay.Record
}

func (s *memorySource) Next() bool {
	if s.next >= len(s.records) {
		return false
	}
	s.current = s.records[s.next]
	s.next++
	return true
}

func (s *memorySource) Record() replay.Record { return s.current }
func (s *memorySource) Err() error            { return nil }
func (s *memorySource) Close() error          { return nil }

// fileLogs reads the files that a replay.FileLog wrote.
type fileLogs struct {
	dir     string
	codecs  *sim.Codecs
	engines []sim.StateID
}

// FileLogs reads the files that a replay.FileLog wrote under dir, which are opened one at a time
// and read as the run goes on.
func FileLogs(dir string, codecs *sim.Codecs) (Logs, error) {
	engines, err := replay.Engines(dir)
	if err != nil {
		return nil, err
	}
	return fileLogs{dir: dir, codecs: codecs, engines: sortedIDs(slices.Values(engines))}, nil
}

func (f fileLogs) Engines() []sim.StateID { return f.engines }

func (f fileLogs) Open(engine sim.StateID) (Source, error) {
	r, err := replay.ReadFileLog(f.dir, engine, 0, f.codecs)
	if err != nil {
		return nil, err
	}
	return r, nil // a *replay.FileLogReader is a Source and a Seeded
}

func sortedIDs(ids func(yield func(sim.StateID) bool)) []sim.StateID {
	out := slices.Collect(ids)
	slices.Sort(out)
	return out
}
