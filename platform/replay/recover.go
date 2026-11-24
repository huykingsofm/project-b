package replay

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
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
func RecoveryLine(dir string, codecs *sim.Codecs) (map[sim.StateID]uint64, error) {
	engines, err := Engines(dir)
	if err != nil {
		return nil, err
	}
	type dependencyAt struct {
		tick uint64
		dep  sim.StateDependency
	}
	limits := map[sim.StateID]uint64{}
	depends := map[sim.StateID][]dependencyAt{}
	for _, engine := range engines {
		if limits[engine], err = segmentlog.LastFrameID(findEngineDir(dir, engine)); err != nil {
			return nil, fmt.Errorf("the log of %v: %w", engine, err)
		}
		r, err := ReadFileLog(dir, engine, 0, codecs)
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

// CutToRecoveryLine cuts the log of each engine under dir to the tick that RecoveryLine says, and
// returns those ticks. What is cut is kept next to the segments, in files that the log does not
// see (see segmentlog.CutAfter), and a log that is already at its tick is not touched. No log may
// be open for writing. After it, each log ends at a tick that no crash takes back, and an engine
// that goes on is made for the log with the tick after it.
func CutToRecoveryLine(dir string, codecs *sim.Codecs) (map[sim.StateID]uint64, error) {
	limits, err := RecoveryLine(dir, codecs)
	if err != nil {
		return nil, err
	}
	for engine, limit := range limits {
		logDir := findEngineDir(dir, engine)
		last, err := segmentlog.LastFrameID(logDir)
		if err != nil {
			return nil, fmt.Errorf("the log of %v: %w", engine, err)
		}
		if last <= limit {
			continue
		}
		if err := segmentlog.CutAfter(logDir, limit); err != nil {
			return nil, fmt.Errorf("the log of %v: %w", engine, err)
		}
	}
	return limits, nil
}
