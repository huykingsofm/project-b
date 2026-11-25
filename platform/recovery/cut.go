package recovery

import (
	"fmt"
	"path/filepath"

	"github.com/huykingsofm/project-b/platform/checkpoint"
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// CutToRecoveryLine cuts the log of each engine under dir to the tick that RecoveryLine says, and
// returns those ticks. The checkpoints of an engine above its tick hold what was cut, so they are
// no longer checkpoints. Nothing is deleted: what is cut of the logs, and the checkpoints that
// are above the line, go to one directory of the archive (`.archive/cut-<n>/<engine>/`), where
// no one reads them as a log or a checkpoint. A log that is already at its tick, and an engine
// with no checkpoint above it, are not touched. The cache of the recovery line is written again
// with the ticks, which are what is safe now. No log may be open for writing. After it, each log
// ends at a tick that no crash takes back, and an engine that goes on is made for the log with
// the tick after it.
func CutToRecoveryLine(dir string, codecs *sim.Codecs) (map[sim.StateID]uint64, error) {
	limits, err := RecoveryLine(dir, codecs)
	if err != nil {
		return nil, err
	}
	cutDir, err := nextCutDir(dir)
	if err != nil {
		return nil, err
	}
	for engine, limit := range limits {
		logDir := replay.EngineDir(dir, engine)
		archive := filepath.Join(cutDir, filepath.Base(logDir))
		last, err := segmentlog.LastFrameID(logDir)
		if err != nil {
			return nil, fmt.Errorf("the log of %v: %w", engine, err)
		}
		if last > limit {
			if err := segmentlog.CutAfter(logDir, limit, archive); err != nil {
				return nil, fmt.Errorf("the log of %v: %w", engine, err)
			}
		}
		if err := archiveCheckpointsAbove(dir, engine, limit, archive); err != nil {
			return nil, fmt.Errorf("the checkpoints of %v: %w", engine, err)
		}
	}
	if err := WriteLineCache(dir, LineCache{Safe: limits}); err != nil {
		return nil, err
	}
	return limits, nil
}

// archiveCheckpointsAbove moves the checkpoints of an engine above the tick to the archive, the
// highest first, so that a crash in the middle leaves checkpoints that are all whole and the cut
// is made again.
func archiveCheckpointsAbove(
	dir string, engine sim.StateID, limit uint64, archive string,
) error {
	ticks, err := checkpoint.Ticks(dir, engine)
	if err != nil {
		return err
	}
	for i := len(ticks) - 1; i >= 0 && ticks[i] > limit; i-- {
		if err := checkpoint.Archive(dir, engine, ticks[i], archive); err != nil {
			return err
		}
	}
	return nil
}
