package recovery

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/logging"
	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// RecoveryLine works out, for each engine that has a log under dir, the last tick of it that a
// recovery keeps. An engine starts at the last tick of its log, and is cut before the first tick
// that depends on a tick of another engine that is cut away, or that has no log at all; this is
// repeated, since the engine that was cut may be what another depends on, until nothing changes.
// What is left is a state of the cluster in which no engine took a message that its sender did not
// send, or read a copy that its source did not publish, and each engine can be made again by
// running its own log, with no other. A tick that is kept is whole: a tick is cut with everything
// that follows it.
//
// The cache of the line (ReadLineCache) says up to which tick of each engine nothing is cut, so the
// dependencies of the ticks after it are all that is read. A cache that says more than a log holds
// is not used, and the logs are read whole.
func RecoveryLine(dir string, codecs *sim.Codecs) (map[sim.StateID]uint64, error) {
	engines, err := replay.Engines(dir)
	if err != nil {
		return nil, err
	}
	limits := map[sim.StateID]uint64{}
	for _, engine := range engines {
		if limits[engine], err = segmentlog.LastFrameID(replay.EngineDir(dir, engine)); err != nil {
			return nil, fmt.Errorf("the log of %v: %w", engine, err)
		}
	}
	known := knownSafe(dir, limits)
	type dependencyAt struct {
		tick uint64
		dep  sim.StateDependency
	}
	depends := map[sim.StateID][]dependencyAt{}
	for _, engine := range engines {
		r, err := replay.ReadFileLog(dir, engine, known[engine]+1, codecs)
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
