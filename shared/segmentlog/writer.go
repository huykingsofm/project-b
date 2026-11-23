package segmentlog

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/huykingsofm/project-b/shared/logging"
)

// Options are the settings of a Writer.
type Options struct {
	// SegmentBytes is the size at which a segment is closed and the next record starts a new one. A
	// segment ends at a batch, so it can be larger by what one batch holds.
	SegmentBytes int64
	// SyncEvery is how often what was written is forced to disk (group commit), whatever the number
	// of batches in that time.
	SyncEvery time.Duration
	// Backlog is how many batches can wait for the goroutine that writes; at least one.
	Backlog int
	// SegmentMeta returns the meta that is written in the header of a new segment. It is given the
	// first payload that the segment will hold, so the meta can say what the segment starts at (a
	// tick, say), and it is called by the goroutine of the Writer, so it must not use what the
	// caller changes. Nil means an empty meta. A meta is at most 64 KiB; a longer one is cut.
	SegmentMeta func(first []byte) []byte
}

// batch is the records of one Commit.
type batch struct {
	buf []byte // the frames of the records
}

// Writer appends records to the segments of a log. Append and Commit only fill and hand over a
// buffer: the file is written, and synced every SyncEvery, by a goroutine of the Writer, so the
// caller never waits for the disk unless that goroutine is more than Backlog batches behind. The
// methods can be called from any goroutine, one at a time; the engine that owns the log calls them
// from its own.
type Writer struct {
	dir  string
	opts Options

	mu             sync.Mutex // protects pending, pendingRecords and closed
	pending        []byte
	pendingRecords int
	closed         bool
	next           uint64 // the index of the next segment; used by the goroutine that writes
	batches        chan batch
	free           chan []byte
	done           chan struct{}
	stalled        atomic.Uint64
	failure        atomic.Pointer[error]
}

// Open opens the log in dir, which it creates when it does not exist. A crash can leave the last
// segment with a record that is not whole, or with no records: Open cuts the first off and removes
// the second, so that what is read afterwards is whole. The segment that the Writer starts is a new
// one, at the tick of the first record it is given.
func Open(dir string, opts Options) (*Writer, error) {
	if opts.SegmentBytes <= 0 || opts.SyncEvery <= 0 {
		return nil, ErrBadOptions
	}
	opts.Backlog = max(opts.Backlog, 1)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := repair(dir); err != nil {
		return nil, err
	}
	segments, err := Segments(dir)
	if err != nil {
		return nil, err
	}
	w := &Writer{
		dir: dir, opts: opts,
		batches: make(chan batch, opts.Backlog),
		free:    make(chan []byte, opts.Backlog+1),
		done:    make(chan struct{}),
	}
	if len(segments) > 0 {
		w.next = segments[len(segments)-1].Index + 1
	}
	go w.run()
	return w, nil
}

// Append adds a record to the buffer of the batch that Commit hands over.
func (w *Writer) Append(payload []byte) error {
	if len(payload) > maxRecord {
		return ErrTooLarge
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return ErrClosed
	}
	w.pending = appendFrame(w.pending, payload)
	w.pendingRecords++
	return nil
}

// Commit hands the records that were appended since the last Commit to the goroutine that writes
// them. It returns the first error that writing the log has had, which is the same for every later
// call: a log that failed does not write anything more.
func (w *Writer) Commit() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return ErrClosed
	}
	w.handOver()
	return w.Err()
}

// handOver sends the pending records, if any. It is called with w.mu held.
func (w *Writer) handOver() {
	if len(w.pending) == 0 {
		return
	}
	// a batch is its records and a commit
	w.pending = appendCommit(w.pending, uint32(w.pendingRecords))
	w.pendingRecords = 0
	b := batch{buf: w.pending}
	select {
	case w.pending = <-w.free:
		w.pending = w.pending[:0]
	default:
		w.pending = nil
	}
	select {
	case w.batches <- b:
	default:
		w.stalled.Add(1)
		// the goroutine that writes is behind: wait for it, a log must not lose records
		w.batches <- b
	}
}

// Stalled is how many times Commit had to wait because the goroutine that writes was behind.
func (w *Writer) Stalled() uint64 { return w.stalled.Load() }

// Err is the first error that writing or syncing the log has had, or nil.
func (w *Writer) Err() error {
	if p := w.failure.Load(); p != nil {
		return *p
	}
	return nil
}

// Close hands over what is pending, waits until everything is written and synced, and returns the
// first error of the log. Later calls return ErrClosed.
func (w *Writer) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return ErrClosed
	}
	w.closed = true
	w.handOver()
	close(w.batches)
	w.mu.Unlock()
	<-w.done
	return w.Err()
}

func (w *Writer) fail(err error) {
	if w.failure.CompareAndSwap(nil, &err) {
		logging.Error("segmentlog failed and writes nothing more", "dir", w.dir, "err", err)
	}
}

// run writes the batches, and forces what is written to disk every SyncEvery.
func (w *Writer) run() {
	defer close(w.done)
	var (
		f     *os.File
		size  int64
		dirty bool
	)
	closeFile := func() {
		if f == nil {
			return
		}
		if dirty {
			if err := f.Sync(); err != nil {
				w.fail(err)
			}
			dirty = false
		}
		if err := f.Close(); err != nil {
			w.fail(err)
		}
		f = nil
	}
	ticker := time.NewTicker(w.opts.SyncEvery)
	defer ticker.Stop()
	for {
		select {
		case b, ok := <-w.batches:
			if !ok {
				closeFile()
				return
			}
			if w.Err() == nil {
				if f != nil && size >= w.opts.SegmentBytes {
					closeFile()
				}
				if f == nil {
					var err error
					if f, size, err = w.create(b.buf); err != nil {
						w.fail(err)
					}
				}
				if f != nil && w.Err() == nil {
					if _, err := f.Write(b.buf); err != nil {
						w.fail(err)
					} else {
						size += int64(len(b.buf))
						dirty = true
					}
				}
			}
			select {
			case w.free <- b.buf[:0]:
			default:
			}
		case <-ticker.C:
			if f != nil && dirty {
				if err := f.Sync(); err != nil {
					w.fail(err)
				}
				dirty = false
			}
		}
	}
}

// create makes the next segment, with its header. The meta is asked for the first payload of the
// batch.
func (w *Writer) create(frames []byte) (*os.File, int64, error) {
	var meta []byte
	if w.opts.SegmentMeta != nil {
		meta = w.opts.SegmentMeta(firstPayload(frames))
		meta = meta[:min(len(meta), maxMeta)]
	}
	f, err := os.OpenFile(
		filepath.Join(w.dir, segmentName(w.next)), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644,
	)
	if err != nil {
		return nil, 0, err
	}
	// the file is not durable until its entry in the directory is
	if err := syncDir(w.dir); err != nil {
		f.Close()
		return nil, 0, err
	}
	header := appendHeader(nil, meta)
	if _, err := f.Write(header); err != nil {
		f.Close()
		return nil, 0, err
	}
	w.next++
	return f, int64(len(header)), nil
}

// firstPayload is the payload of the first record of a batch of frames.
func firstPayload(frames []byte) []byte {
	if len(frames) < frameHead {
		return nil
	}
	n := int(binary.LittleEndian.Uint32(frames))
	return frames[frameHead:min(frameHead+n, len(frames))]
}
