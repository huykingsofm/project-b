package replay

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
)

// copyKey names a copy of an engine: whose, and the tick it was taken at.
type copyKey struct {
	source sim.StateID
	tick   uint64
}

// index is what a replay has to know of the whole of the logs before it runs the first tick, kept
// small: a number for each message and a count for each copy that is read. It is made by one pass
// over each log, from the tick an engine starts at.
type index struct {
	// maxQueueSize is how many commands or messages the busiest tick of any engine takes in, and so
	// how large a queue has to be for the replay to put them all in before the tick runs.
	maxQueueSize int
	// copyReads is how many reads there are of each copy: the copy is kept until all have been
	// made.
	copyReads map[copyKey]int
	// msgsToBeReturned says which messages came back undelivered.
	msgsToBeReturned map[msgKey]bool
	// inboundMessageHash is the hash of each message as the log of its receiver took it (Inbound):
	// the sender, replayed, has to send the same.
	inboundMessageHash map[msgKey]uint64
}

// buildIndex reads each log once. It returns an error when a log cannot be read and when a log
// was made with another seed than the one of the builder.
func buildIndex(
	logs Logs, seed uint64, queueSize int, starts map[sim.StateID]uint64,
) (*index, error) {
	idx := &index{
		maxQueueSize: queueSize, copyReads: map[copyKey]int{},
		msgsToBeReturned: map[msgKey]bool{}, inboundMessageHash: map[msgKey]uint64{},
	}
	for _, address := range logs.Engines() {
		if err := idx.add(logs, address, seed, starts[address]); err != nil {
			return nil, fmt.Errorf("the log of %v: %w", address, err)
		}
	}
	return idx, nil
}

// add reads the log of an engine that starts at the tick start. What the records of the ticks up
// to start say is in the checkpoint it starts from, so they are not looked at.
func (idx *index) add(logs Logs, address sim.StateID, seed, start uint64) error {
	src, err := logs.Open(address, start)
	if err != nil {
		return err
	}
	defer src.Close()
	var tick uint64
	// what the engine takes in at the tick: commands, messages, messages that came back
	takenMsgs := 0
	first := true
	for src.Next() {
		rec := src.Record()
		if first {
			first = false
			if s, ok := src.(eventlog.Seeded); ok && s.Seed() != seed {
				return fmt.Errorf(
					"it was made with the seed %d and the builder has %d", s.Seed(), seed,
				)
			}
		}
		if rec.Tick <= start {
			continue
		}
		if rec.Tick != tick {
			tick, takenMsgs = rec.Tick, 0
		}
		switch body := rec.Body.(type) {
		case eventlog.Inbound:
			idx.inboundMessageHash[keyOf(body.Message.Head())] = eventlog.HashMessage(body.Message)
			takenMsgs++
		case eventlog.Command:
			takenMsgs++
		case eventlog.Returned:
			idx.msgsToBeReturned[keyOf(body.Header)] = true
			takenMsgs++
		case eventlog.StableRead:
			if body.CopyTick > 0 {
				idx.copyReads[copyKey{body.Source, body.CopyTick}]++
			}
		}
		// all of them are in the queue before the tick runs
		idx.maxQueueSize = max(idx.maxQueueSize, takenMsgs+1)
	}
	return src.Err()
}
