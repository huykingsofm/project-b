package checkpoint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
)

// A checkpoint is the file `ckpt-<tick>.snap` in the directory of its engine, the tick in 20
// digits so that the names sort as the ticks do. It is written to `ckpt-<tick>.snap.tmp`, synced,
// and renamed, so a file with the name of a checkpoint is always whole; a temporary file that a
// crash left is not a checkpoint, and is written over when the same tick is written again.
const (
	namePrefix = "ckpt-"
	nameSuffix = ".snap"
	tmpSuffix  = ".tmp"
)

// fileName is the name of the checkpoint of an engine at the tick.
func fileName(tick uint64) string {
	return fmt.Sprintf("%s%020d%s", namePrefix, tick, nameSuffix)
}

// Path is the file of the checkpoint of an engine at the tick, under the directory of a cluster.
func Path(dir string, engine sim.StateID, tick uint64) string {
	return filepath.Join(replay.EngineDir(dir, engine), fileName(tick))
}

// Ticks lists the ticks at which an engine has a checkpoint under the directory of a cluster, in
// rising order. A file that is not named as a checkpoint is not one; an engine with no directory
// has none.
func Ticks(dir string, engine sim.StateID) ([]uint64, error) {
	entries, err := os.ReadDir(replay.EngineDir(dir, engine))
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

// writeFile writes the checkpoint of an engine at the tick: to a temporary file, which is synced
// and then renamed, and the directory is synced after it, so that the checkpoint is on the disk
// whole when it returns, or not there at all.
func writeFile(dir string, engine sim.StateID, tick uint64, data []byte) error {
	engineDir := replay.EngineDir(dir, engine)
	if err := os.MkdirAll(engineDir, 0o755); err != nil {
		return err
	}
	path := Path(dir, engine, tick)
	tmp := path + tmpSuffix
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDir(engineDir)
}

// syncDir syncs a directory, so that a file renamed in it stays there after a crash.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	if closeErr := d.Close(); err == nil {
		err = closeErr
	}
	return err
}
