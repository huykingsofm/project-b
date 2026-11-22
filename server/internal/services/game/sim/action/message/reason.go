package message

// Reason says why a message could not be delivered.
type Reason uint8

// The reasons.
const (
	// Retired: the engine was removed.
	Retired Reason = iota + 1
	// NotBuilt: the engine does not exist yet, or the address names none.
	NotBuilt
	// Moved: what the message is about has gone to another engine.
	Moved
	// Stopping: the engine, or the world, is stopping.
	Stopping
	// Full: the inbox of the engine is full.
	Full
)

func (r Reason) String() string {
	switch r {
	case Retired:
		return "retired"
	case NotBuilt:
		return "not built"
	case Moved:
		return "moved"
	case Stopping:
		return "stopping"
	case Full:
		return "full"
	}
	return "unknown"
}
