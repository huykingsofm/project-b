package replay

import (
	"sync"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// handedTicks is the ticks of the frames that were handed to a segment log writer, in order: the
// writer says how many batches are on the disk, and a batch is a tick, so this says which tick that
// is. It is added to by the goroutine of the engine and read by whoever asks up to which tick the
// log is safe.
type handedTicks struct {
	mu      sync.Mutex
	ticks   []uint64 // the ticks of the batches that are not known to be on the disk
	settled uint64   // how many batches were let go of, which were on the disk
	last    uint64   // the tick of the last of them, or 0
}

// Add says that the frame of the tick was handed over.
func (h *handedTicks) Add(tick uint64) {
	h.mu.Lock()
	h.ticks = append(h.ticks, tick)
	h.mu.Unlock()
}

// Durable is the tick of the last frame that is on the disk, given how many batches are: the ones
// before it are let go of, and 0 is answered while there is none. It never goes back.
func (h *handedTicks) Durable(synced uint64) uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	for h.settled < synced && len(h.ticks) > 0 {
		h.last = h.ticks[0]
		h.ticks = h.ticks[1:]
		h.settled++
	}
	return h.last
}

// lastFrameTickOf is the tick of the last frame in the log in dir, or 0 when it has none.
func lastFrameTickOf(dir string) (uint64, error) {
	r, err := segmentlog.OpenReader(dir, func([]byte) bool { return true }) // from the last segment
	if err != nil {
		return 0, err
	}
	defer r.Close()
	var last uint64
	for r.Next() {
		tick, err := payloadTick(r.Payload())
		if err != nil {
			return 0, err
		}
		last = tick
	}
	return last, r.Err()
}

// Durable is the tick up to which what an engine recorded is on the disk, so that a crash loses
// nothing at or before it: the tick of the last frame that was synced, or the last tick of the log
// that was there when it was opened, whichever is later. It is 0 for an engine with no log. It
// reads an atomic and a short list, and is safe to call from any goroutine.
func (f *FileLog) Durable(engine sim.StateID) uint64 {
	l, err := f.log(engine)
	if err != nil {
		return 0
	}
	return l.Durable()
}
