package replay

import (
	"sync"

	"github.com/huykingsofm/project-b/platform/sim"
)

// tickDependencies is what an engine depends on at a tick of its own.
type tickDependencies struct {
	tick uint64
	deps []sim.StateDependency
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
	d.entries = append(d.entries, tickDependencies{tick: tick, deps: deps})
	d.mu.Unlock()
}

// Safe is the last tick of the engine that is safe: at most the durable tick, and before the first
// tick that depends on a tick of another engine that is not safe, as safeOf says of it. A tick all
// of whose dependencies are safe is let go of, since they stay safe.
func (d *dependencyList) Safe(durable uint64, safeOf func(sim.StateID) uint64) uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	limit, done := durable, 0
	for done < len(d.entries) && d.entries[done].tick <= durable {
		blocked := false
		for _, dep := range d.entries[done].deps {
			if dep.Tick > safeOf(dep.Source) {
				blocked = true
				break
			}
		}
		if blocked {
			limit = d.entries[done].tick - 1
			break
		}
		done++
	}
	d.entries = append(d.entries[:0], d.entries[done:]...)
	return limit
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
// depends on a tick of another waits for that engine, and that engine may wait for another.
func (f *FileLog) settle() {
	var logs []*engineLog
	f.engines.Range(func(_, v any) bool {
		logs = append(logs, v.(*engineLog))
		return true
	})
	safeOf := func(id sim.StateID) uint64 {
		if v, ok := f.engines.Load(id); ok {
			return v.(*engineLog).safe.Load()
		}
		return 0
	}
	for changed := true; changed; {
		changed = false
		for _, l := range logs {
			if safe := l.dependencies.Safe(l.Durable(), safeOf); safe > l.safe.Load() {
				l.safe.Store(safe)
				changed = true
			}
		}
	}
}

// watch works out what is safe each time that a log has synced, until Close.
func (f *FileLog) watch() {
	defer close(f.watching)
	for {
		select {
		case <-f.synced:
			f.settle()
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
