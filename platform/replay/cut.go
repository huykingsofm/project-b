package replay

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/huykingsofm/project-b/platform/checkpoint"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/segmentlog"
)

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

// archiveDirName is the directory under the one of the cluster that holds what a recovery cut: the
// segments of the logs and the checkpoints that are above the recovery line. It starts with a dot,
// so that no one reads it as the log of an engine.
const archiveDirName = ".archive"

// cutDirPrefix starts the name of the directory of one cut: `cut-<n>`, n counting from 1.
const cutDirPrefix = "cut-"

// nextCutDir is the directory of the next cut of the cluster in dir: one above the largest that is
// there, so that what two cuts keep never meets. It is not made here: what is cut makes it.
func nextCutDir(dir string) (string, error) {
	root := filepath.Join(dir, archiveDirName)
	entries, err := os.ReadDir(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	last := uint64(0)
	for _, e := range entries {
		n, err := strconv.ParseUint(strings.TrimPrefix(e.Name(), cutDirPrefix), 10, 64)
		if e.IsDir() && strings.HasPrefix(e.Name(), cutDirPrefix) && err == nil {
			last = max(last, n)
		}
	}
	return filepath.Join(root, fmt.Sprintf("%s%d", cutDirPrefix, last+1)), nil
}
