package replay

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"

	"github.com/huykingsofm/project-b/platform/checkpoint"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/logging"
	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// Plan says what a replay does: the tick that each engine is run to (its limit) and the tick that
// it starts at (the one of the checkpoint it is made from, or 0). The limits are the recovery line
// of the logs: for each engine, the last tick of its log that no crash takes back. Every engine
// that is kept took only what another sent and read only what another published, so that each can
// be made again by running its own log, with no other. A plan is made by MakePlan or PlanFrom
// only, so that a replay always runs on logs that are whole in that way.
//
// A plan is not changed after it is made. Reading it changes nothing on the disk: Cut does, and a
// recovery that goes on live needs it.
type Plan struct {
	limits      map[sim.StateID]uint64
	starts      map[sim.StateID]uint64
	logs        Logs
	checkpoints CheckpointSource
	dir         string // the directory of the files; empty when the logs are not files
}

// Limits is the last tick of each engine that is kept: the recovery line. An engine that is not in
// it has no log.
func (p *Plan) Limits() map[sim.StateID]uint64 { return maps.Clone(p.limits) }

// Starts is the tick each engine starts at: the tick of its checkpoint, or 0.
func (p *Plan) Starts() map[sim.StateID]uint64 { return maps.Clone(p.starts) }

// MakePlan makes the plan of the files under dir. It reads the logs, and the checkpoints to choose
// from; nothing is written. The cache of the line, when it holds, saves reading the logs whole. No
// log may be open for writing.
func MakePlan(dir string, codecs *sim.Codecs) (*Plan, error) {
	logs, err := eventlog.NewFileLogs(dir, codecs)
	if err != nil {
		return nil, err
	}
	last, err := lastTicks(logs)
	if err != nil {
		return nil, err
	}
	limits, err := recoveryLine(logs, last, knownSafe(dir, last))
	if err != nil {
		return nil, err
	}
	starts, err := chooseStarts(dir, logs, limits)
	if err != nil {
		return nil, err
	}
	return &Plan{
		limits: limits, starts: starts, logs: logs, dir: dir,
		checkpoints: fileCheckpoints{dir: dir, starts: starts},
	}, nil
}

// PlanFrom makes the plan of logs that are not files (in memory, or as a test makes them): the
// engines start at starts and are made from the checkpoints that checkpoints gives, and from the
// record that begins their log where it gives none; the starts are not above the limits. Such a
// plan cannot be cut.
func PlanFrom(
	logs Logs, starts map[sim.StateID]uint64, checkpoints CheckpointSource,
) (*Plan, error) {
	last, err := lastTicks(logs)
	if err != nil {
		return nil, err
	}
	limits, err := recoveryLine(logs, last, nil)
	if err != nil {
		return nil, err
	}
	for engine, start := range starts {
		if start > limits[engine] {
			return nil, fmt.Errorf(
				"the engine %v starts at the tick %d, above its limit %d",
				engine, start, limits[engine],
			)
		}
	}
	return &Plan{
		limits: limits, starts: maps.Clone(starts), logs: logs, checkpoints: checkpoints,
	}, nil
}

// Cut brings the files under the directory of the plan to the plan: the log of each engine is cut
// to its limit. The checkpoints of an engine above its limit hold what was cut, so they are no
// longer checkpoints. Nothing is deleted: what is cut of the logs, and the checkpoints that are
// above the line, go to one directory of the archive (`.archive/cut-<n>/<engine>/`), where no one
// reads them as a log or a checkpoint. A log that is already at its limit, and an engine with no
// checkpoint above it, are not touched. The cache of the recovery line is written again with the
// limits, which are what is safe now. No log may be open for writing. After it, each log ends at a
// tick that no crash takes back, and an engine that goes on is made for the log with the tick
// after it. A plan made from logs that are not files cannot be cut.
func (p *Plan) Cut() error {
	if p.dir == "" {
		return errors.New("a plan made from logs that are not files cannot be cut")
	}
	cutDir, err := nextCutDir(p.dir)
	if err != nil {
		return err
	}
	for engine, limit := range p.limits {
		logDir := eventlog.EngineDir(p.dir, engine)
		archive := filepath.Join(cutDir, filepath.Base(logDir))
		last, err := segmentlog.LastFrameID(logDir)
		if err != nil {
			return fmt.Errorf("the log of %v: %w", engine, err)
		}
		if last > limit {
			if err := segmentlog.CutAfter(logDir, limit, archive); err != nil {
				return fmt.Errorf("the log of %v: %w", engine, err)
			}
		}
		if err := archiveCheckpointsAbove(p.dir, engine, limit, archive); err != nil {
			return fmt.Errorf("the checkpoints of %v: %w", engine, err)
		}
	}
	return WriteLineCache(p.dir, LineCache{Safe: p.limits})
}

// lastTicks is the last tick of the log of each engine that has one.
func lastTicks(logs Logs) (map[sim.StateID]uint64, error) {
	last := map[sim.StateID]uint64{}
	for _, engine := range logs.Engines() {
		tick, err := logs.Last(engine)
		if err != nil {
			return nil, fmt.Errorf("the log of %v: %w", engine, err)
		}
		last[engine] = tick
	}
	return last, nil
}

// recoveryLine works out, for each engine that has a log, the last tick of it that a recovery
// keeps (last is the last tick of each log). An engine starts at the last tick of its log, and is
// cut before the first tick that depends on a tick of another engine that is cut away, or that has
// no log at all; this is repeated, since the engine that was cut may be what another depends on,
// until nothing changes. What is left is a state of the cluster in which no engine took a message
// that its sender did not send, or read a copy that its source did not publish. A tick that is kept
// is whole: a tick is cut with everything that follows it.
//
// known is the tick of each engine up to which nothing is cut (the cache of the line), so the
// dependencies of the ticks after it are all that is read; nil reads the logs whole.
func recoveryLine(
	logs Logs, last, known map[sim.StateID]uint64,
) (map[sim.StateID]uint64, error) {
	engines := logs.Engines()
	limits := maps.Clone(last)
	type dependencyAt struct {
		tick uint64
		dep  sim.StateDependency
	}
	depends := map[sim.StateID][]dependencyAt{}
	for _, engine := range engines {
		r, err := logs.Open(engine, known[engine]+1)
		if err != nil {
			return nil, fmt.Errorf("the log of %v: %w", engine, err)
		}
		for r.Next() {
			rec := r.Record()
			if dep, ok := rec.Dependency(); ok {
				depends[engine] = append(depends[engine], dependencyAt{tick: rec.Tick, dep: dep})
			}
		}
		err = r.Err()
		r.Close()
		if err != nil {
			return nil, fmt.Errorf("the log of %v: %w", engine, err)
		}
	}
	for changed := true; changed; {
		changed = false
		for _, engine := range engines {
			for _, d := range depends[engine] {
				if d.tick > limits[engine] {
					break
				}
				if d.dep.Tick > limits[d.dep.Source] { // an engine with no log has a limit of 0
					limits[engine], changed = d.tick-1, true
					break
				}
			}
		}
	}
	return limits, nil
}

// knownSafe is the tick of each engine up to which the cache says nothing is cut, and 0 for an
// engine it says nothing of. A cache that cannot be read, or that says more than a log holds
// (last is the last tick of each log), is logged and not used: the logs are read whole.
func knownSafe(dir string, last map[sim.StateID]uint64) map[sim.StateID]uint64 {
	cache, found, err := ReadLineCache(dir)
	if err != nil {
		logging.Warn("the cache of the recovery line is not used", "dir", dir, "err", err)
		return nil
	}
	if !found {
		return nil
	}
	for engine, safe := range cache.Safe {
		if end, has := last[engine]; !has || safe > end {
			logging.Warn(
				"the cache of the recovery line is not used: it says more than the log holds",
				"dir", dir, "engine", engine, "safe", safe, "log_ends_at", end, "has_log", has,
			)
			return nil
		}
	}
	return cache.Safe
}

// chooseStarts says the tick each engine of limits starts at: the tick of its latest checkpoint at
// or below its limit, or 0 when it has none. An engine runs its log from there, and reads the copy
// of another engine at a tick the engine it reads from must be able to give: the one it was made
// at, or one that it runs up to. So an engine that is read at a tick before its own start starts at
// an earlier checkpoint, which is repeated until none moves: a start only goes down, so it ends.
func chooseStarts(
	dir string, logs Logs, limits map[sim.StateID]uint64,
) (map[sim.StateID]uint64, error) {
	starts := map[sim.StateID]uint64{}
	reads := map[sim.StateID][]copyKey{}
	engines := slices.Sorted(maps.Keys(limits))
	for _, engine := range engines {
		start, err := latestCheckpoint(dir, engine, limits[engine])
		if err != nil {
			return nil, err
		}
		starts[engine] = start
		if reads[engine], err = copyReads(logs, engine, start, limits[engine]); err != nil {
			return nil, err
		}
	}
	for changed := true; changed; {
		changed = false
		for _, engine := range engines {
			for _, read := range reads[engine] {
				source := read.source
				if read.tick == 0 || starts[source] <= read.tick {
					continue
				}
				// the reads of the source between its new start and its old one are to be made now
				start, err := latestCheckpoint(dir, source, read.tick)
				if err != nil {
					return nil, err
				}
				more, err := copyReads(logs, source, start, starts[source])
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
func copyReads(logs Logs, engine sim.StateID, after, to uint64) ([]copyKey, error) {
	r, err := logs.Open(engine, after+1)
	if err != nil {
		return nil, fmt.Errorf("the log of %v: %w", engine, err)
	}
	defer r.Close()
	var out []copyKey
	for r.Next() {
		rec := r.Record()
		if rec.Tick > to {
			break
		}
		if read, ok := rec.Body.(eventlog.StableRead); ok {
			out = append(out, copyKey{source: read.Source, tick: read.CopyTick})
		}
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("the log of %v: %w", engine, err)
	}
	return out, nil
}

// CheckpointSource says which checkpoint each engine of a replay starts from: the bytes of the
// checkpoint (EngineCheckpoint.Encode) that the engine is made from, and false when it has none:
// the engine starts at the tick 0, from the record that begins its log. An error is one that the
// checkpoint could not be read.
type CheckpointSource interface {
	Checkpoint(engine sim.StateID) ([]byte, bool, error)
}

// fileCheckpoints is the CheckpointSource of the checkpoint files under dir: for each engine that
// starts above tick 0 in starts, the file of its checkpoint at that tick. An engine that is not in
// starts, or is at 0, has none.
type fileCheckpoints struct {
	dir    string
	starts map[sim.StateID]uint64
}

func (f fileCheckpoints) Checkpoint(id sim.StateID) ([]byte, bool, error) {
	start := f.starts[id]
	if start == 0 {
		return nil, false, nil
	}
	data, err := checkpoint.Read(f.dir, id, start)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}
