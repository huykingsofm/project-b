package replay

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
)

// Logs is where the records of a run come from: a source for each engine that recorded something,
// which can be opened as often as needed. A replay reads each once to index it and once to run it.
// A source gives the records of the tick from and of the ticks after it, as an engine that starts
// from a checkpoint needs no more. eventlog.FileLogs, eventlog.MemoryLogs and eventlog.MemLog are
// Logs.
type Logs interface {
	Engines() []sim.StateID
	// Last is the last tick of the log of the engine, 0 when it holds none.
	Last(engine sim.StateID) (uint64, error)
	Open(engine sim.StateID, from uint64) (eventlog.Source, error)
}

// tickReader reads the records of one engine a tick at a time, in the order of the ticks, and holds
// the first record of the next tick while it waits for it.
type tickReader struct {
	src   eventlog.Source
	ahead eventlog.Record
	have  bool
}

func newTickReader(src eventlog.Source) *tickReader {
	c := &tickReader{src: src}
	c.advance()
	return c
}

func (c *tickReader) advance() { c.have = c.src.Next(); c.ahead = c.src.Record() }

// take returns the records of the tick, which must be asked for in rising order, one after another.
// A record of an earlier tick than the one asked for means the log is not in the order of its
// ticks.
func (c *tickReader) take(tick uint64) ([]eventlog.Record, error) {
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

func (c *tickReader) close() error { return c.src.Close() }
