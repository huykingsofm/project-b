package replay

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/huykingsofm/project-b/platform/journal/checkpoint"
	"github.com/huykingsofm/project-b/platform/sim"
)

// archiveDirName is the directory under the one of the cluster that holds what a recovery cut: the
// segments of the logs and the checkpoints that are above the recovery line. It starts with a dot,
// so that no one reads it as the log of an engine.
const archiveDirName = ".archive"

// cutDirPrefix starts the name of the directory of one cut: `cut-<n>`, n counting from 1.
const cutDirPrefix = "cut-"

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
