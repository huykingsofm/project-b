package testkit

import (
	"sync"

	"github.com/huykingsofm/project-b/platform/sim"
)

// Recorder is an engine.Emitter that keeps the events of every engine, for a test to count.
type Recorder struct {
	mu     sync.Mutex
	events []sim.Event
}

// Emit keeps the events.
func (r *Recorder) Emit(_ sim.StateID, _ uint64, events []sim.Event) {
	r.mu.Lock()
	r.events = append(r.events, events...)
	r.mu.Unlock()
}

// Count returns how many events of the type T were emitted, by every engine.
func (r *Recorder) Count[T any]() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.events {
		if _, ok := e.(T); ok {
			n++
		}
	}
	return n
}
