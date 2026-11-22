package runner

import (
	"fmt"

	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
)

// copyKey names a copy of an engine: whose, and the tick it was taken at.
type copyKey struct {
	source message.Address
	tick   uint64
}

// index is what the runner has to know of the whole of the logs before it runs the first tick,
// kept small: a number for each message, a count for each copy that is read, and the last tick of
// each engine. It is made by one pass over each log.
type index struct {
	// last is the last tick that each engine has to run: the last tick of its own log, or of a copy
	// of it that another engine reads.
	last map[message.Address]uint64
	// queue is how many commands or messages the busiest tick of any engine takes in, and so how
	// large a queue has to be for the replay to put them all in before the tick runs.
	queue int
	// uses is how many reads there are of each copy: the copy is kept until all have been made.
	uses map[copyKey]int
	// returned says which messages came back undelivered.
	returned map[msgKey]bool
	// want is the hash of each message as its receiver took it: the sender, replayed, has to make
	// the same.
	want map[msgKey]uint64
}

// buildIndex reads each log once. It returns an error when a log cannot be read and when a log
// was made with another seed than the one of the builder.
func buildIndex(logs Logs, seed uint64, queue int) (*index, error) {
	idx := &index{
		last: map[message.Address]uint64{}, queue: queue, uses: map[copyKey]int{},
		returned: map[msgKey]bool{}, want: map[msgKey]uint64{},
	}
	for _, address := range logs.Engines() {
		if err := idx.add(logs, address, seed); err != nil {
			return nil, fmt.Errorf("the log of %v: %w", address, err)
		}
	}
	return idx, nil
}

func (idx *index) add(logs Logs, address message.Address, seed uint64) error {
	src, err := logs.Open(address)
	if err != nil {
		return err
	}
	defer src.Close()
	var tick uint64
	taken := 0 // what the engine takes in at the tick: commands, messages, messages that came back
	first := true
	for src.Next() {
		rec := src.Record()
		if first {
			first = false
			if s, ok := src.(Seeded); ok && s.Seed() != seed {
				return fmt.Errorf(
					"it was made with the seed %d and the builder has %d", s.Seed(), seed,
				)
			}
		}
		if rec.Tick != tick {
			tick, taken = rec.Tick, 0
		}
		idx.last[address] = max(idx.last[address], rec.Tick)
		switch body := rec.Body.(type) {
		case replay.Inbound:
			idx.want[keyOf(body.Message.Head())] = replay.HashMessage(body.Message)
			taken++
		case replay.Command:
			taken++
		case replay.Returned:
			idx.returned[keyOf(body.Header)] = true
			taken++
		case replay.StableRead:
			idx.last[body.Source] = max(idx.last[body.Source], body.CopyTick)
			if body.CopyTick > 0 {
				idx.uses[copyKey{body.Source, body.CopyTick}]++
			}
		}
		idx.queue = max(idx.queue, taken+1) // all of them are in the queue before the tick runs
	}
	return src.Err()
}
