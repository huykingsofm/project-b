package eventlog

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/huykingsofm/project-b/platform/sim"

	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/logging"
	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// formatVersion is the version of the records and of the meta of a segment of the event log.
const formatVersion codec.Version = 1

// FileLogOptions are the settings of a FileLog.
type FileLogOptions struct {
	// Seed is the seed of the cluster, written in the meta of every segment: a replay needs it.
	Seed uint64
	// SegmentBytes, SyncEvery and Backlog are those of segmentlog.Options.
	SegmentBytes int64
	SyncEvery    time.Duration
	Backlog      int
	// OnSafe is told what is safe for every engine each time it moves, and once more when the log
	// is closed. It is called on the goroutine that works out what is safe, so it must not wait
	// long. Nil: no one is told.
	OnSafe SafeObserver
}

// SafeObserver is told what a FileLog says is safe (see FileLog.Safe), for every engine that has a
// log. final is true for the last call, made when the log is closed and everything is on the disk.
// A recovery reads its cache of the recovery line from what it is told.
type SafeObserver interface {
	SafeChanged(safe map[sim.StateID]uint64, final bool)
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
// batch: `[record]...`, a record being `[length][kind][body]`. The tick of a frame is its ID in the
// segment log, which counts the frames of the log and keeps the ID of the first frame of each
// segment in its header, so the frame does not hold it. Every tick has a frame, an empty one when
// the engine recorded nothing, so the batches of a log are its ticks one by one. The meta of a
// segment is the version, the engine and the seed.
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

// NewFileLog makes a event log that writes under dir. Nothing is created until an engine records.
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
			"the event log could not record and is not enough to replay from",
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
		Meta:   segmentMeta(engine, f.opts.Seed),
		OnSync: f.wake,
	})
	l := &engineLog{w: w, openErr: err}
	// what is in the log already is on the disk, and the engine goes on after its last frame
	var opened uint64
	if err == nil {
		opened = w.LastID()
	}
	l.safe.Store(opened)
	l.last.Store(opened)
	f.engines.Store(engine, l)
	f.started.Do(func() {
		f.running = true
		go f.watch()
	})
	// reported here, once: record does not report it again for each record
	if err != nil && f.failure == nil {
		f.failure = err
		logging.Error(
			"the event log could not record and is not enough to replay from",
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

// Safe is the tick of the engine up to which a crash takes back nothing it did and nothing it took:
// what it recorded is on the disk, and so is what each message that it took from another engine,
// and each copy of another engine that it read, says that engine did. It is behind Durable by what
// the engines it depends on have not synced yet, and it never goes back. It is 0 for an engine
// that has no log, and it is safe to call from any goroutine.
func (f *FileLog) Safe(engine sim.StateID) uint64 {
	l, err := f.log(engine)
	if err != nil {
		return 0
	}
	return l.safe.Load()
}

// settle works out again what is safe for every engine, until none changes: an engine whose tick
// depends on a tick of another waits for that engine, and that engine may wait for another. It
// tells the observer of the log when something moved, and always when final.
func (f *FileLog) settle(final bool) {
	var engineLogs []*engineLog
	var ids []sim.StateID
	f.engines.Range(func(k, v any) bool {
		engineLogs = append(engineLogs, v.(*engineLog))
		ids = append(ids, k.(sim.StateID))
		return true
	})
	safeOf := func(id sim.StateID) uint64 {
		if v, ok := f.engines.Load(id); ok {
			return v.(*engineLog).safe.Load()
		}
		return 0
	}
	moved := false
	for changed := true; changed; {
		changed = false
		for _, l := range engineLogs {
			if safe := l.dependencies.Safe(l.Durable(), safeOf); safe > l.safe.Load() {
				l.safe.Store(safe)
				changed, moved = true, true
			}
		}
	}
	if f.opts.OnSafe == nil || !(moved || final) {
		return
	}
	safe := make(map[sim.StateID]uint64, len(ids))
	for i, id := range ids {
		safe[id] = engineLogs[i].safe.Load()
	}
	f.opts.OnSafe.SafeChanged(safe, final)
}

// watch works out what is safe each time that a log has synced, until Close.
func (f *FileLog) watch() {
	defer close(f.watching)
	for {
		select {
		case <-f.synced:
			f.settle(false)
		case <-f.stop:
			return
		}
	}
}

// wake tells the goroutine that works out what is safe that a log has synced.
func (f *FileLog) wake() {
	select {
	case f.synced <- struct{}{}:
	default: // one that is waiting for it already
	}
}

// Durable is the tick up to which what an engine recorded is on the disk, so that a crash loses
// nothing at or before it: the tick of the last frame that was synced, which counts the last tick
// of the log that was there when it was opened. It is 0 for an engine with no log. It reads an
// atomic, and is safe to call from any goroutine.
func (f *FileLog) Durable(engine sim.StateID) uint64 {
	l, err := f.log(engine)
	if err != nil {
		return 0
	}
	return l.Durable()
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
		f.settle(true) // every log is on the disk now
	})
	if err := f.Err(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
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
	tick    uint64        // the tick of the records so far
	started bool          // whether there are records of the tick so far

	dependencies dependencyList        // what the engine depends on, at the ticks not safe yet
	tickDeps     []sim.StateDependency // what it depends on at the tick that is being recorded
	safe         atomic.Uint64         // see FileLog.Safe
	last         atomic.Uint64         // the tick of the last frame, written or not
}

// Durable is the tick up to which what the engine recorded is on the disk: the ID of the last frame
// that the segment log says is, since the ID of a frame is its tick.
func (l *engineLog) Durable() uint64 {
	if l.w == nil {
		return 0
	}
	return l.w.Synced()
}

// OpenError is why the log of the engine could not be opened, or nil.
func (l *engineLog) OpenError() error { return l.openErr }

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

// End hands the frame of the tick to the goroutine that writes: the records, as one record of the
// segment log, which has the tick as its ID, and one batch. The frame is empty when nothing was
// recorded.
func (l *engineLog) End(tick uint64) error {
	if l.started && tick != l.tick {
		l.records.B, l.started, l.tickDeps = l.records.B[:0], false, nil
		return fmt.Errorf("a record of the tick %d is in the frame of the tick %d", l.tick, tick)
	}
	if last := l.last.Load(); tick != last+1 {
		l.records.B, l.started, l.tickDeps = l.records.B[:0], false, nil
		return fmt.Errorf("the tick %d does not follow the tick %d, the last of the log",
			tick, last)
	}
	id, err := l.w.Append(l.records.B)
	l.records.B, l.started = l.records.B[:0], false
	if err != nil {
		return err
	}
	if id != tick { // the segment log counts the frames and the checks above count the ticks
		return fmt.Errorf("the frame of the tick %d has the ID %d in the log", tick, id)
	}
	if err := l.w.AsyncCommit(); err != nil {
		return err
	}
	l.last.Store(tick)
	if len(l.tickDeps) > 0 {
		l.dependencies.Add(tick, l.tickDeps)
		l.tickDeps = nil
	}
	return nil
}

// segmentMeta is the meta of a segment: the format, the ID of the engine and the seed.
func segmentMeta(engine sim.StateID, seed uint64) []byte {
	var w binary.Writer
	w.Uvarint(uint64(formatVersion))
	w.Uvarint(uint64(engine))
	w.Fixed64(seed)
	return w.B
}

// segmentInfo is what the meta of a segment says.
type segmentInfo struct {
	Version codec.Version // of the data of the records of the segment
	Engine  sim.StateID
	Seed    uint64
}

func parseSegmentMeta(meta []byte) (segmentInfo, error) {
	r := binary.NewReader(meta)
	version := r.Uint[codec.Version]()
	if r.Err() == nil && version != formatVersion {
		return segmentInfo{}, fmt.Errorf(
			"the event log is of format %d, this code reads format %d", version, formatVersion,
		)
	}
	info := segmentInfo{
		Version: version,
		Engine:  r.Uint[sim.StateID](),
		Seed:    r.Fixed64(),
	}
	return info, r.Err()
}
