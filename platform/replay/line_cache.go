package replay

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/atomicfile"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/logging"
)

// lineCacheFile is the name of the file of the cache, in the directory of the cluster. It is not a
// directory with a dash in its name, so no one reads it as the log of an engine.
const lineCacheFile = "recovery-line"

// lineCacheVersion is the version of the data of the file.
const lineCacheVersion = 1

// lineCacheEvery is how far an engine has to be ahead of what the cache says of it, in ticks,
// before the cache is written again. It is the period of a checkpoint: a recovery then reads the
// log from a tick that is about as far back as its checkpoint is, and no more.
var lineCacheEvery = config.NewVar(
	"replay.line_cache_every_ticks", 600, config.Range(1, 100000000),
)

// LineCache is what a FileLog said was safe for each engine (replay.FileLog.Safe) at some time.
// What is safe is never cut by a recovery, so the recovery line of an engine is at or above it, and
// a recovery looks at the dependencies of the ticks after it only. It is a bound, not a fact about
// the logs as they are now: it is read after a check that it does not say more than a log holds.
type LineCache struct {
	Safe map[sim.StateID]uint64
}

func (c LineCache) encode() []byte {
	var w binary.Writer
	w.Uvarint(lineCacheVersion)
	ids := slices.Sorted(maps.Keys(c.Safe))
	w.Uvarint(uint64(len(ids)))
	for _, id := range ids {
		w.Uvarint(uint64(id))
		w.Uvarint(c.Safe[id])
	}
	return w.B
}

func decodeLineCache(data []byte) (LineCache, error) {
	r := binary.NewReader(data)
	if version := r.Uvarint(); r.Err() == nil && version != lineCacheVersion {
		return LineCache{}, fmt.Errorf("the cache is of the data version %d", version)
	}
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return LineCache{}, err
	}
	if n > uint64(r.Left()) { // an engine takes at least two bytes
		return LineCache{}, fmt.Errorf("%w: %d engines", binary.ErrShort, n)
	}
	c := LineCache{Safe: make(map[sim.StateID]uint64, n)}
	for range n {
		id := r.Uint[sim.StateID]()
		if _, twice := c.Safe[id]; twice {
			return LineCache{}, fmt.Errorf("%w: the engine %d is twice", binary.ErrBadValue, id)
		}
		c.Safe[id] = r.Uvarint()
	}
	if err := r.Err(); err != nil {
		return LineCache{}, err
	}
	if r.Left() != 0 {
		return LineCache{}, fmt.Errorf("%d bytes after the cache", r.Left())
	}
	return c, nil
}

// ReadLineCache reads the cache of the cluster in dir, and says false when there is none.
func ReadLineCache(dir string) (LineCache, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, lineCacheFile))
	if errors.Is(err, os.ErrNotExist) {
		return LineCache{}, false, nil
	}
	if err != nil {
		return LineCache{}, false, err
	}
	c, err := decodeLineCache(data)
	if err != nil {
		return LineCache{}, false, fmt.Errorf("the cache of the recovery line: %w", err)
	}
	return c, true, nil
}

// WriteLineCache writes the cache of the cluster in dir, whole or not at all.
func WriteLineCache(dir string, c LineCache) error {
	return atomicfile.Write(filepath.Join(dir, lineCacheFile), c.encode())
}

// ConfigSource is where the variables of the configuration are read from.
type ConfigSource interface {
	Snapshot() *config.Snapshot
}

// LineCacheWriter writes the cache of the recovery line of a cluster each time a FileLog says that
// some engine has gone `replay.line_cache_every_ticks` ticks past what the cache says of it, and
// once more when the log is closed. It is the replay.SafeObserver of the FileLog of the cluster,
// and is called by one goroutine at a time, so it needs no lock.
type LineCacheWriter struct {
	dir     string
	cfg     ConfigSource
	written map[sim.StateID]uint64
}

// NewLineCacheWriter makes the writer of the cache of the cluster in dir. A nil cfg reads the
// default of the period.
func NewLineCacheWriter(dir string, cfg ConfigSource) *LineCacheWriter {
	return &LineCacheWriter{dir: dir, cfg: cfg}
}

// SafeChanged writes the cache when it is due, or when final. A cache that cannot be written is
// logged, and the next one is tried when it is due: a recovery without it reads more, and nothing
// is lost.
func (w *LineCacheWriter) SafeChanged(safe map[sim.StateID]uint64, final bool) {
	if !final && !w.due(safe) {
		return
	}
	if err := WriteLineCache(w.dir, LineCache{Safe: safe}); err != nil {
		logging.Error(
			"the cache of the recovery line could not be written", "dir", w.dir, "err", err,
		)
		return
	}
	w.written = maps.Clone(safe)
}

// due says whether an engine is a period past what was written.
func (w *LineCacheWriter) due(safe map[sim.StateID]uint64) bool {
	var snapshot *config.Snapshot
	if w.cfg != nil {
		snapshot = w.cfg.Snapshot()
	}
	every := uint64(lineCacheEvery.Get(snapshot))
	for id, tick := range safe {
		if tick >= w.written[id]+every {
			return true
		}
	}
	return false
}
