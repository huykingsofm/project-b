// Package enginetest holds what the tests of the engines and of what runs them again share: the
// world they run, the recorder of what the engines tell their clients, a replay log that writes in
// memory and to files, and a scenario that runs a cluster through every kind of input.
package enginetest

import (
	"sync"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	worldevent "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world/event"
	zoneevent "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone/event"
	zonelayerevent "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer/event"
)

// Recorder keeps what the engines give to the output.
type Recorder struct {
	mu        sync.Mutex
	world     []worldevent.Event
	zone      []zoneevent.Event
	zonelayer []zonelayerevent.Event
}

func (r *Recorder) EmitWorld(_ message.Address, _ uint64, c []worldevent.Event) {
	r.mu.Lock()
	r.world = append(r.world, c...)
	r.mu.Unlock()
}

func (r *Recorder) EmitZone(_ message.Address, _ uint64, c []zoneevent.Event) {
	r.mu.Lock()
	r.zone = append(r.zone, c...)
	r.mu.Unlock()
}

func (r *Recorder) EmitZoneLayer(_ message.Address, _ uint64, c []zonelayerevent.Event) {
	r.mu.Lock()
	r.zonelayer = append(r.zonelayer, c...)
	r.mu.Unlock()
}

// countChanges counts the events of type T.
func countChanges[T any, C any](r *Recorder, events []C) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range events {
		if _, ok := any(c).(T); ok {
			n++
		}
	}
	return n
}

// CountWorld, CountZone and CountZoneLayer count the events of type T that the engines of a scope
// gave.
func (r *Recorder) CountWorld[T any]() int     { return countChanges[T](r, r.world) }
func (r *Recorder) CountZone[T any]() int      { return countChanges[T](r, r.zone) }
func (r *Recorder) CountZoneLayer[T any]() int { return countChanges[T](r, r.zonelayer) }
