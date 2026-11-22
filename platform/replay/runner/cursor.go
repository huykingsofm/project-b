package runner

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/replay"
)

// cursor reads the records of one engine a tick at a time, in the order of the ticks, and holds
// the first record of the next tick while it waits for it.
type cursor struct {
	src   Source
	ahead replay.Record
	have  bool
}

func newCursor(src Source) *cursor {
	c := &cursor{src: src}
	c.advance()
	return c
}

func (c *cursor) advance() { c.have = c.src.Next(); c.ahead = c.src.Record() }

// take returns the records of the tick, which must be asked for in rising order, one after another.
// A record of an earlier tick than the one asked for means the log is not in the order of its
// ticks.
func (c *cursor) take(tick uint64) ([]replay.Record, error) {
	var out []replay.Record
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
