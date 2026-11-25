package eventlog

import (
	"sync"

	"github.com/huykingsofm/project-b/platform/sim"
)

// tickDependencies is what an engine depends on at a tick of its own.
type tickDependencies struct {
	sourceTick uint64
	deps       []sim.StateDependency
}

// dependencyList is what the engine of a log depends on, tick by tick, for the ticks that are not
// known to be safe yet. The goroutine of the engine adds to it, and the goroutine that works out
// what is safe reads and takes from it.
type dependencyList struct {
	mu      sync.Mutex
	entries []tickDependencies // in the order of the ticks
}

// Add says what the engine depends on at a tick.
func (d *dependencyList) Add(tick uint64, deps []sim.StateDependency) {
	d.mu.Lock()
	d.entries = append(d.entries, tickDependencies{sourceTick: tick, deps: deps})
	d.mu.Unlock()
}

// Safe is the last tick of the engine that is safe: at most the durable tick, and before the first
// tick that depends on a tick of another engine that is not safe, as safeOf says of it. A tick all
// of whose dependencies are safe is let go of, since they stay safe.
func (d *dependencyList) Safe(durable uint64, safeOf func(sim.StateID) uint64) uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	safe, n := durable, 0
	for n < len(d.entries) && d.entries[n].sourceTick <= durable {
		blocked := false
		for _, dep := range d.entries[n].deps {
			if dep.Tick > safeOf(dep.Source) {
				blocked = true
				break
			}
		}
		if blocked {
			safe = d.entries[n].sourceTick - 1
			break
		}
		n++
	}
	d.entries = append(d.entries[:0], d.entries[n:]...)
	return safe
}
