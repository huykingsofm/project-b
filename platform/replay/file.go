package replay

import (
	"errors"
	"fmt"
	"github.com/huykingsofm/project-b/platform/sim"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/logging"
	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// formatVersion is the version of the records and of the meta of a segment of the replay log.
const formatVersion codec.Version = 2

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
// What an engine writes in a tick is one frame, which is one record of the segment log and so one
// batch: `[tick][record]...`, a record being `[length][kind][body]`. Every tick has a frame, an
// empty one when the engine recorded nothing, so the batches of a log are its ticks one by one.
// The meta of a segment is the version, the engine, the seed and the tick of its first frame.
type FileLog struct {
	dir  string
	opts FileLogOptions

	mu      sync.Mutex // protects opening a log; the logs of the engines are then read without it
	engines sync.Map   // sim.StateID -> *engineLog
	failure error      // protected by mu

	synced   chan struct{} // a log has synced: what is safe is worked out again
	stop     chan struct{}
	watching chan struct{} // closed when the goroutine that works out what is safe ends
	started  sync.Once
	running  bool // whether that goroutine was started; protected by mu
	closed   sync.Once
}

// engineLog is the log of one engine: the frame of the tick that is being recorded, and the writer
// that the frames go to.
type engineLog struct {
	w *segmentlog.Writer // nil when the log of the engine could not be opened
	// why it could not: the engine does not try again, as a writer that failed does not
	openErr error

	// The next fields are used by the goroutine of the engine only.
	records binary.Writer // the records of the tick, each with its length
	scratch binary.Writer // one record, while it is written
	frame   binary.Writer // the frame, while it is handed over
	tick    uint64        // the tick of the records so far
	started bool          // whether there are records of the tick so far

	handed handedTicks // the ticks of the frames handed over
	opened uint64      // the tick of the last frame that was in the log when it was opened

	dependencies dependencyList        // what the engine depends on, at the ticks not safe yet
	tickDeps     []sim.StateDependency // what it depends on at the tick that is being recorded
	safe         atomic.Uint64         // see FileLog.Safe
	last         atomic.Uint64         // the tick of the last frame; see FileLog.Last
}

// Durable is the tick up to which what the engine recorded is on the disk.
func (l *engineLog) Durable() uint64 {
	if l.w == nil {
		return 0
	}
	return max(l.opened, l.handed.Durable(l.w.Synced()))
}

// OpenError is why the log of the engine could not be opened, or nil.
func (l *engineLog) OpenError() error { return l.openErr }

// Usable reports whether the log of the engine was opened.
func (l *engineLog) Usable() bool { return l.w != nil }

// WriteError is the first error that writing the log has had, or nil.
func (l *engineLog) WriteError() error {
	if l.w == nil {
		return nil
	}
	return l.w.Err()
}

// Close writes and syncs what was handed over.
func (l *engineLog) Close() error {
	if l.w == nil {
		return nil
	}
	return l.w.Close()
}

// Add puts a record in the frame of the tick. A record that cannot be encoded is left out, and the
// error says why.
func (l *engineLog) Add(tick uint64, body Body) error {
	if l.started && tick != l.tick {
		return fmt.Errorf("a record of the tick %d is in the frame of the tick %d", tick, l.tick)
	}
	l.scratch.B = l.scratch.B[:0]
	if err := encodeRecord(&l.scratch, body); err != nil {
		return err
	}
	l.records.Bytes(l.scratch.B)
	l.tick, l.started = tick, true
	if dep, ok := (Record{Body: body}).Dependency(); ok {
		l.tickDeps = append(l.tickDeps, dep)
	}
	return nil
}

// End hands the frame of the tick to the goroutine that writes: the tick and then the records, as
// one record of the segment log and one batch. The frame is empty when nothing was recorded.
func (l *engineLog) End(tick uint64) error {
	if l.started && tick != l.tick {
		l.records.B, l.started, l.tickDeps = l.records.B[:0], false, nil
		return fmt.Errorf("a record of the tick %d is in the frame of the tick %d", l.tick, tick)
	}
	if last := l.last.Load(); last != 0 && tick != last+1 {
		l.records.B, l.started, l.tickDeps = l.records.B[:0], false, nil
		return fmt.Errorf("the tick %d does not follow the tick %d, the last of the log",
			tick, last)
	}
	l.frame.B = l.frame.B[:0]
	l.frame.Uvarint(tick)
	l.frame.B = append(l.frame.B, l.records.B...)
	l.records.B, l.started = l.records.B[:0], false
	if err := l.w.Append(l.frame.B); err != nil {
		return err
	}
	if err := l.w.Commit(); err != nil {
		return err
	}
	l.handed.Add(tick)
	l.last.Store(tick)
	if len(l.tickDeps) > 0 {
		l.dependencies.Add(tick, l.tickDeps)
		l.tickDeps = nil
	}
	return nil
}

// NewFileLog makes a replay log that writes under dir. Nothing is created until an engine records.
func NewFileLog(dir string, opts FileLogOptions) *FileLog {
	return &FileLog{
		dir: dir, opts: opts,
		synced: make(chan struct{}, 1), stop: make(chan struct{}), watching: make(chan struct{}),
	}
}

// engineDirName is the name of the directory of the log of an engine: the name of the type of its
// state (see sim.MapStateTypeToName) and the number of the state among the states of its type. The
// full ID is in the meta of the segments, which is what tells two states of different types with
// the same number apart.
func engineDirName(engine sim.StateID) string {
	return fmt.Sprintf("%s-%d", engine.Type(), engine.Local())
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
		if e := l.OpenError(); e != nil {
			err = e
			return false
		}
		if e := l.WriteError(); e != nil {
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
		return l, l.OpenError()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.engines.Load(engine); ok {
		l := v.(*engineLog)
		return l, l.OpenError()
	}
	logDir := filepath.Join(f.dir, engineDirName(engine))
	w, err := segmentlog.Open(logDir, segmentlog.Options{
		SegmentBytes: f.opts.SegmentBytes, SyncEvery: f.opts.SyncEvery, Backlog: f.opts.Backlog,
		SegmentMeta: func(first []byte) []byte {
			return segmentMeta(engine, f.opts.Seed, first)
		},
		OnSync: func(uint64) { f.wake() },
	})
	l := &engineLog{w: w, openErr: err}
	if err == nil {
		// what is in the log already is on the disk, and the engine goes on after it
		if l.opened, err = lastFrameTickOf(logDir); err != nil {
			l.w.Close()
			l.w, l.openErr = nil, err
		}
	}
	l.safe.Store(l.opened)
	l.last.Store(l.opened)
	f.engines.Store(engine, l)
	f.started.Do(func() {
		f.running = true
		go f.watch()
	})
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

// Append puts a record in the frame of the tick of the engine.
func (f *FileLog) Append(engine sim.StateID, tick uint64, body Body) {
	l, err := f.log(engine)
	if err != nil {
		return // the log of the engine could not be opened; log() reported it
	}
	if err = l.Add(tick, body); err != nil {
		f.fail(engine, err)
	}
}

// EndTick hands the frame of the tick to the goroutine that writes it.
func (f *FileLog) EndTick(engine sim.StateID, tick uint64) {
	l, err := f.log(engine)
	if err != nil {
		return // the log of the engine could not be opened; log() reported it
	}
	if err = l.End(tick); err != nil {
		f.fail(engine, err)
	}
}

// Close writes and syncs everything that was recorded, and returns the first error of the log.
func (f *FileLog) Close() error {
	var errs []error
	f.engines.Range(func(_, v any) bool {
		l := v.(*engineLog)
		if err := l.Close(); err != nil && !errors.Is(err, segmentlog.ErrClosed) {
			errs = append(errs, err)
		}
		return true
	})
	f.closed.Do(func() {
		close(f.stop)
		f.mu.Lock()
		running := f.running
		f.mu.Unlock()
		if running {
			<-f.watching
		}
		f.settle() // every log is on the disk now
	})
	if err := f.Err(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// segmentMeta is the meta of a segment that starts with the payload first: the format, the ID of
// the engine, the seed and the tick of that record.
func segmentMeta(engine sim.StateID, seed uint64, first []byte) []byte {
	var w binary.Writer
	w.Uvarint(uint64(formatVersion))
	w.Uvarint(uint64(engine))
	w.Fixed64(seed)
	tick, _ := payloadTick(first)
	w.Uvarint(tick)
	return w.B
}

// segmentInfo is what the meta of a segment says.
type segmentInfo struct {
	Version   codec.Version // of the data of the records of the segment
	Engine    sim.StateID
	Seed      uint64
	FirstTick uint64
}

func parseSegmentMeta(meta []byte) (segmentInfo, error) {
	r := binary.NewReader(meta)
	version := r.Uint[codec.Version]()
	if r.Err() == nil && version != formatVersion {
		return segmentInfo{}, fmt.Errorf(
			"the replay log is of format %d, this code reads format %d", version, formatVersion,
		)
	}
	info := segmentInfo{
		Version:   version,
		Engine:    r.Uint[sim.StateID](),
		Seed:      r.Fixed64(),
		FirstTick: r.Uvarint(),
	}
	return info, r.Err()
}

// Last is the tick of the last frame of the log of the engine, whether it is on the disk or not, or
// 0 when it has none.
func (f *FileLog) Last(engine sim.StateID) uint64 {
	l, err := f.log(engine)
	if err != nil {
		return 0
	}
	return l.last.Load()
}
