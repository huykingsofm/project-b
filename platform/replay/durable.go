package replay

import (
	"github.com/huykingsofm/project-b/platform/sim"
)

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
