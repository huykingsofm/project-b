package replay

import (
	"errors"
	"fmt"
	"github.com/huykingsofm/project-b/platform/sim"
	"path/filepath"
	"sync"
	"time"

	"github.com/huykingsofm/project-b/shared/binarycodec"
	"github.com/huykingsofm/project-b/shared/logging"
	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// formatVersion is the version of the records and of the meta of a segment of the replay log.
const formatVersion binarycodec.Version = 1

// FileLogOptions are the settings of a FileLog.
type FileLogOptions struct {
	// Seed is the seed of the cluster, written in the meta of every segment: a replay needs it.
	Seed uint64
	// SegmentBytes, SyncEvery and Backlog are those of segmentlog.Options.
	SegmentBytes int64
	SyncEvery    time.Duration
	Backlog      int
}

// FileLog is a Log that writes to files, one log for each engine, in the directory
// `<dir>/<name>-<id>/` of the engine (see segmentlog). Each engine writes only to its own log with
// no lock shared with the others: the first record of an engine opens its log, and the records of a
// tick are kept in a buffer that EndTick hands to the goroutine that writes. So the methods of an
// engine must be called from one goroutine at a time, which they are: they are called by the engine
// itself.
//
// A record that cannot be written (an action with no number, a command with no codec) is not
// written, and the log is marked as failed: Err returns the first error, which is also logged. A
// log that failed is not enough to replay from.
//
// Records are the bytes `[kind][tick][body]`, with the whole tick in each so that a record does not
// depend on the segment it is in. The meta of a segment is the version, the engine, the
// seed and the tick of its first record.
type FileLog struct {
	dir  string
	opts FileLogOptions

	mu      sync.Mutex // protects opening a log; the logs of the engines are then read without it
	engines sync.Map   // sim.StateID -> *engineLog
	names   sync.Map   // sim.StateID -> string
	failure error      // protected by mu
}

// engineLog is the log of one engine.
type engineLog struct {
	w *segmentlog.Writer // nil when the log of the engine could not be opened
	// why it could not: the engine does not try again, as a writer that failed does not
	err     error
	scratch binarycodec.Writer // used by the goroutine of the engine only
}

// NewFileLog makes a replay log that writes under dir. Nothing is created until an engine records.
func NewFileLog(dir string, opts FileLogOptions) *FileLog {
	return &FileLog{dir: dir, opts: opts}
}

// engineDirName is the name of the directory of the log of an engine: the name of the type of its
// state and the number of the state among the states of its type. The full ID is in the meta of
// the segments, which is what tells two states of different types with the same number apart.
func engineDirName(name string, engine sim.StateID) string {
	return fmt.Sprintf("%s-%d", name, engine.Local())
}

// Describe tells the log what the type of the state of an engine is called.
func (f *FileLog) Describe(engine sim.StateID, name string) { f.names.Store(engine, name) }

// nameOf is what the engine told the log the type of its state is called. An engine that told
// nothing is called "engine".
func (f *FileLog) nameOf(engine sim.StateID) string {
	if v, ok := f.names.Load(engine); ok {
		return v.(string)
	}
	return "engine"
}

// Err is the first error that the log has had, or nil.
func (f *FileLog) Err() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failure != nil {
		return f.failure
	}
	var err error
	f.engines.Range(func(_, v any) bool {
		l := v.(*engineLog)
		if l.err != nil {
			err = l.err
			return false
		}
		if e := l.w.Err(); e != nil {
			err = e
			return false
		}
		return true
	})
	return err
}

func (f *FileLog) fail(engine sim.StateID, err error) {
	f.mu.Lock()
	first := f.failure == nil
	if first {
		f.failure = err
	}
	f.mu.Unlock()
	if first {
		logging.Error(
			"the replay log could not record and is not enough to replay from",
			"engine",
			engine,
			"err",
			err,
		)
	}
}

// log returns the log of an engine, opening it the first time.
func (f *FileLog) log(engine sim.StateID) (*engineLog, error) {
	if v, ok := f.engines.Load(engine); ok {
		l := v.(*engineLog)
		return l, l.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.engines.Load(engine); ok {
		l := v.(*engineLog)
		return l, l.err
	}
	logDir := filepath.Join(f.dir, engineDirName(f.nameOf(engine), engine))
	w, err := segmentlog.Open(logDir, segmentlog.Options{
		SegmentBytes: f.opts.SegmentBytes, SyncEvery: f.opts.SyncEvery, Backlog: f.opts.Backlog,
		SegmentMeta: func(first []byte) []byte {
			return segmentMeta(engine, f.opts.Seed, first)
		},
	})
	l := &engineLog{w: w, err: err}
	f.engines.Store(engine, l)
	// reported here, once: record does not report it again for each record
	if err != nil && f.failure == nil {
		f.failure = err
		logging.Error(
			"the replay log could not record and is not enough to replay from",
			"engine",
			engine,
			"err",
			err,
		)
	}
	return l, err
}

// record appends one record to the buffer of the tick of the engine.
func (f *FileLog) record(engine sim.StateID, r Record) {
	l, err := f.log(engine)
	if err != nil {
		return // the log of the engine could not be opened; log() reported it
	}
	l.scratch.B = l.scratch.B[:0]
	if err = encodeRecord(&l.scratch, r); err == nil {
		err = l.w.Append(l.scratch.B)
	}
	if err != nil {
		f.fail(engine, err)
	}
}

// Wants reports true for every kind: a FileLog writes what it is given.
func (f *FileLog) Wants(Kind) bool { return true }

func (f *FileLog) Append(engine sim.StateID, tick uint64, body Body) {
	f.record(engine, Record{Tick: tick, Body: body})
}

// EndTick hands the records of the tick to the goroutine that writes them.
func (f *FileLog) EndTick(engine sim.StateID, _ uint64) {
	v, ok := f.engines.Load(engine)
	if !ok {
		return // the engine recorded nothing yet
	}
	l := v.(*engineLog)
	if l.w == nil {
		return // the log of the engine could not be opened, and the failure was reported
	}
	if err := l.w.Commit(); err != nil {
		f.fail(engine, err)
	}
}

// Close writes and syncs everything that was recorded, and returns the first error of the log.
func (f *FileLog) Close() error {
	var errs []error
	f.engines.Range(func(_, v any) bool {
		l := v.(*engineLog)
		if l.w == nil {
			return true
		}
		if err := l.w.Close(); err != nil && !errors.Is(err, segmentlog.ErrClosed) {
			errs = append(errs, err)
		}
		return true
	})
	if err := f.Err(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// segmentMeta is the meta of a segment that starts with the payload first: the format, the ID of
// the engine, the seed and the tick of that record.
func segmentMeta(engine sim.StateID, seed uint64, first []byte) []byte {
	var w binarycodec.Writer
	w.Uvarint(uint64(formatVersion))
	w.Uvarint(uint64(engine))
	w.Fixed64(seed)
	tick, _ := payloadTick(first)
	w.Uvarint(tick)
	return w.B
}

// segmentInfo is what the meta of a segment says.
type segmentInfo struct {
	Version   binarycodec.Version // of the data of the records of the segment
	Engine    sim.StateID
	Seed      uint64
	FirstTick uint64
}

func parseSegmentMeta(meta []byte) (segmentInfo, error) {
	r := binarycodec.NewReader(meta)
	version := binarycodec.Version(r.Uvarint())
	if r.Err() == nil && version != formatVersion {
		return segmentInfo{}, fmt.Errorf(
			"the replay log is of format %d, this code reads format %d", version, formatVersion,
		)
	}
	info := segmentInfo{
		Version:   version,
		Engine:    sim.StateID(r.Uvarint()),
		Seed:      r.Fixed64(),
		FirstTick: r.Uvarint(),
	}
	return info, r.Err()
}
