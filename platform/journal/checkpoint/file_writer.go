package checkpoint

import (
	"sync"
	"sync/atomic"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/logging"
)

// offeredCheckpoint is a checkpoint that an engine took and that waits to be written.
type offeredCheckpoint struct {
	engine sim.StateID
	tick   uint64
	encode func() ([]byte, error)
}

// FileWriter writes the checkpoints of the engines of a cluster to files under its directory, one
// after the other, on one goroutine of its own: the engines hand it what they took at the end of a
// tick and go on at once. It holds a bounded queue; a checkpoint that finds the queue full is
// dropped, and so is one that comes after Close: the next one of that engine comes a period
// later. What is dropped, and a checkpoint that cannot be encoded or written, is logged and
// counted, and the engines are not told: it is never worth stopping a tick for.
type FileWriter struct {
	// where it writes
	dir string

	// the queue of the checkpoints that engines offer: mu protects closed against the queue being
	// closed while an engine offers, and done closes when the goroutine has written what was in it
	mu     sync.RWMutex
	closed bool
	queue  chan offeredCheckpoint
	done   chan struct{}

	// how many checkpoints were written, dropped and failed
	written atomic.Uint64
	dropped atomic.Uint64
	failed  atomic.Uint64
}

// NewFileWriter makes a writer of the checkpoints of a cluster under dir, with room for queue
// checkpoints that wait to be written, and starts its goroutine. Close stops it.
func NewFileWriter(dir string, queue int) *FileWriter {
	w := &FileWriter{
		dir: dir, queue: make(chan offeredCheckpoint, queue), done: make(chan struct{}),
	}
	go w.run()
	return w
}

// Offer hands a checkpoint of an engine to be written, without waiting. encode makes its bytes and
// is called on the goroutine of the writer. A checkpoint that finds the queue full, or the writer
// closed, is dropped and logged.
func (w *FileWriter) Offer(engine sim.StateID, tick uint64, encode func() ([]byte, error)) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.closed {
		w.drop(engine, tick, "the writer of the checkpoints is closed")
		return
	}
	select {
	case w.queue <- offeredCheckpoint{engine: engine, tick: tick, encode: encode}:
	default:
		w.drop(engine, tick, "the queue of the checkpoints to write is full")
	}
}

// Close writes the checkpoints that wait in the queue and returns when they are written. A
// checkpoint offered after it is dropped. It is called once, after the engines have stopped.
func (w *FileWriter) Close() {
	w.mu.Lock()
	if !w.closed {
		w.closed = true
		close(w.queue)
	}
	w.mu.Unlock()
	<-w.done
}

// Written is how many checkpoints the writer has written.
func (w *FileWriter) Written() uint64 { return w.written.Load() }

// Dropped is how many checkpoints the writer did not take: the queue was full, or it was closed.
func (w *FileWriter) Dropped() uint64 { return w.dropped.Load() }

// Failed is how many checkpoints could not be encoded or written.
func (w *FileWriter) Failed() uint64 { return w.failed.Load() }

// drop counts and logs a checkpoint that is not written.
func (w *FileWriter) drop(engine sim.StateID, tick uint64, why string) {
	w.dropped.Add(1)
	logging.Warn("a checkpoint is dropped", "engine", engine, "tick", tick, "why", why)
}

// run writes the checkpoints of the queue, in the order they came, until the queue is closed and
// empty.
func (w *FileWriter) run() {
	defer close(w.done)
	for c := range w.queue {
		w.write(c)
	}
}

// write encodes a checkpoint and writes its file. One that cannot be is logged here, where it
// ends, and counted.
func (w *FileWriter) write(c offeredCheckpoint) {
	data, err := c.encode()
	if err == nil {
		err = writeFile(w.dir, c.engine, c.tick, data)
	}
	if err != nil {
		w.failed.Add(1)
		logging.Error(
			"a checkpoint could not be written", "engine", c.engine, "tick", c.tick, "err", err,
		)
		return
	}
	w.written.Add(1)
}
