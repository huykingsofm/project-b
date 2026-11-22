package runner

import (
	"cmp"
	"maps"
	"slices"

	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
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
	Engines() []message.Address
	Open(engine message.Address) (Source, error)
}

// Seeded is a Source that knows the seed of the run it was recorded in, as the files of a
// replay.FileLog do. The runner refuses a log whose seed is not the one of the builder.
type Seeded interface {
	Seed() uint64
}

// MemoryLogs holds the records of each engine in memory.
type MemoryLogs map[message.Address][]replay.Record

// Engines returns the engines that have records, in the order of their addresses.
func (m MemoryLogs) Engines() []message.Address { return sortedAddresses(maps.Keys(m)) }

// Open returns a source of the records of the engine.
func (m MemoryLogs) Open(engine message.Address) (Source, error) {
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
	engines []message.Address
}

// FileLogs reads the files that a replay.FileLog wrote under dir, which are opened one at a time
// and read as the run goes on.
func FileLogs(dir string) (Logs, error) {
	engines, err := replay.Engines(dir)
	if err != nil {
		return nil, err
	}
	return fileLogs{dir: dir, engines: sortedAddresses(slices.Values(engines))}, nil
}

func (f fileLogs) Engines() []message.Address { return f.engines }

func (f fileLogs) Open(engine message.Address) (Source, error) {
	r, err := replay.ReadFileLog(f.dir, engine, 0)
	if err != nil {
		return nil, err
	}
	return r, nil // a *replay.FileLogReader is a Source and a Seeded
}

func sortedAddresses(addresses func(yield func(message.Address) bool)) []message.Address {
	out := slices.Collect(addresses)
	slices.SortFunc(out, func(a, b message.Address) int {
		return cmp.Or(cmp.Compare(a.Scope, b.Scope), cmp.Compare(a.ID, b.ID))
	})
	return out
}
