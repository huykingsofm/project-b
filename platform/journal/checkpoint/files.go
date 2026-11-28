package checkpoint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/atomicfile"
)

// A checkpoint is the file `ckpt-<tick>.snap` in the directory of its engine, the tick in 20
// digits so that the names sort as the ticks do. It is written with atomicfile, so a file with the
// name of a checkpoint is always whole; a temporary file that a crash left is not a checkpoint, and
// is written over when the same tick is written again.
const (
	namePrefix = "ckpt-"
	nameSuffix = ".snap"
)

// Path is the file of the checkpoint of an engine at the tick, under the directory of a cluster.
func Path(dir string, engine sim.StateID, tick uint64) string {
	return filepath.Join(eventlog.EngineDir(dir, engine), fileName(tick))
}

// Ticks lists the ticks at which an engine has a checkpoint under the directory of a cluster, in
// rising order. A file that is not named as a checkpoint is not one; an engine with no directory
// has none.
func Ticks(dir string, engine sim.StateID) ([]uint64, error) {
	entries, err := os.ReadDir(eventlog.EngineDir(dir, engine))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []uint64
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, namePrefix) ||
			!strings.HasSuffix(name, nameSuffix) {
			continue
		}
		tick, err := strconv.ParseUint(
			strings.TrimSuffix(strings.TrimPrefix(name, namePrefix), nameSuffix), 10, 64,
		)
		if err != nil || name != fileName(tick) {
			continue
		}
		out = append(out, tick)
	}
	slices.Sort(out)
	return out, nil
}

// Read returns the bytes of the checkpoint of an engine at the tick.
func Read(dir string, engine sim.StateID, tick uint64) ([]byte, error) {
	return os.ReadFile(Path(dir, engine, tick))
}

// LatestAtOrBelow is the tick of the latest checkpoint that an engine has at or below tick, and
// false when it has none.
func LatestAtOrBelow(dir string, engine sim.StateID, tick uint64) (uint64, bool, error) {
	ticks, err := Ticks(dir, engine)
	if err != nil {
		return 0, false, err
	}
	i, _ := slices.BinarySearch(ticks, tick+1) // the first tick above the one asked for
	if i == 0 {
		return 0, false, nil
	}
	return ticks[i-1], true, nil
}

// Archive moves the checkpoint of an engine at the tick to archiveDir, under its name with the time
// of the move before the extension (atomicfile.Stamped), so that two moves never meet. archiveDir
// is made when it is needed and must be on the same volume as dir: the checkpoint is no longer one,
// and is not deleted. The directories are synced, so that a crash does not bring it back.
func Archive(dir string, engine sim.StateID, tick uint64, archiveDir string) error {
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return err
	}
	from := Path(dir, engine, tick)
	to := filepath.Join(archiveDir, atomicfile.Stamped(fileName(tick), time.Now()))
	if err := os.Rename(from, to); err != nil {
		return err
	}
	if err := atomicfile.SyncDir(archiveDir); err != nil {
		return err
	}
	return atomicfile.SyncDir(filepath.Dir(from))
}

// fileName is the name of the checkpoint of an engine at the tick.
func fileName(tick uint64) string {
	return fmt.Sprintf("%s%020d%s", namePrefix, tick, nameSuffix)
}

// writeFile writes the checkpoint of an engine at the tick, whole or not at all (see atomicfile).
func writeFile(dir string, engine sim.StateID, tick uint64, data []byte) error {
	return atomicfile.Write(Path(dir, engine, tick), data)
}
