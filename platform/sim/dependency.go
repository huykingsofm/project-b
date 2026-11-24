package sim

// StateDependency says that what an engine did at a tick depends on what another engine did at a
// tick: the engine took a message that the other sent at that tick, or read a copy of the state of
// the other that was taken at that tick. What the engine did cannot be told to have happened
// unless the other engine's tick is not lost, and a recovery cuts an engine before the first tick
// whose dependency is on a tick of its source that was lost.
type StateDependency struct {
	// Source is the engine that is depended on.
	Source StateID
	// Tick is the tick of Source that is depended on.
	Tick uint64
}
