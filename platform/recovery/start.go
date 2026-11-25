package recovery

import (
	"fmt"
	"maps"
	"slices"

	"github.com/huykingsofm/project-b/platform/checkpoint"
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
)

// copyRead is a copy of another engine that an engine read at a tick after its start.
type copyRead struct {
	source   sim.StateID
	copyTick uint64
}

// chooseStarts says the tick each engine of limits starts at: the tick of its latest checkpoint at
// or below its limit, or 0 when it has none. An engine runs its log from there, and reads the copy
// of another engine at a tick the engine it reads from must be able to give: the one it was made
// at, or one that it runs up to. So an engine that is read at a tick before its own start starts at
// an earlier checkpoint, which is repeated until none moves: a start only goes down, so it ends.
func chooseStarts(
	dir string, codecs *sim.Codecs, limits map[sim.StateID]uint64,
) (map[sim.StateID]uint64, error) {
	starts := map[sim.StateID]uint64{}
	reads := map[sim.StateID][]copyRead{}
	engines := slices.Sorted(maps.Keys(limits))
	for _, engine := range engines {
		start, err := latestCheckpoint(dir, engine, limits[engine])
		if err != nil {
			return nil, err
		}
		starts[engine] = start
		if reads[engine], err = copyReads(dir, engine, start, limits[engine], codecs); err != nil {
			return nil, err
		}
	}
	for changed := true; changed; {
		changed = false
		for _, engine := range engines {
			for _, read := range reads[engine] {
				source := read.source
				if read.copyTick == 0 || starts[source] <= read.copyTick {
					continue
				}
				// the reads of the source between its new start and its old one are to be made now
				start, err := latestCheckpoint(dir, source, read.copyTick)
				if err != nil {
					return nil, err
				}
				more, err := copyReads(dir, source, start, starts[source], codecs)
				if err != nil {
					return nil, err
				}
				reads[source] = append(reads[source], more...)
				starts[source], changed = start, true
			}
		}
	}
	return starts, nil
}

// latestCheckpoint is the tick of the latest checkpoint of the engine at or below tick, or 0.
func latestCheckpoint(dir string, engine sim.StateID, tick uint64) (uint64, error) {
	at, _, err := checkpoint.LatestAtOrBelow(dir, engine, tick)
	if err != nil {
		return 0, fmt.Errorf("the checkpoints of %v: %w", engine, err)
	}
	return at, nil
}

// copyReads are the copies that the engine read at the ticks after `after` and up to `to`.
func copyReads(
	dir string, engine sim.StateID, after, to uint64, codecs *sim.Codecs,
) ([]copyRead, error) {
	r, err := replay.ReadFileLog(dir, engine, after+1, codecs)
	if err != nil {
		return nil, fmt.Errorf("the log of %v: %w", engine, err)
	}
	defer r.Close()
	var out []copyRead
	for r.Next() {
		rec := r.Record()
		if rec.Tick > to {
			break
		}
		if read, ok := rec.Body.(replay.StableRead); ok {
			out = append(out, copyRead{source: read.Source, copyTick: read.CopyTick})
		}
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("the log of %v: %w", engine, err)
	}
	return out, nil
}
