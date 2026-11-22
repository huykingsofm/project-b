package engine

import "github.com/huykingsofm/project-b/platform/sim"

// Emitter is where an engine gives what changed in its state in a tick, for the clients: the events
// of the tick, in the order they happened, and the ID of the engine and the tick they came from. It
// is called by the goroutine of the engine, between the plan and the publish, so it must not wait.
// What an event is made of belongs to the game, which interprets it.
type Emitter interface {
	Emit(from sim.StateID, tick uint64, events []sim.Event)
}

// NopEmitter discards the events.
type NopEmitter struct{}

// Emit discards the events.
func (NopEmitter) Emit(sim.StateID, uint64, []sim.Event) {}
