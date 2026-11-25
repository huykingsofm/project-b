package engine

import "github.com/huykingsofm/project-b/platform/sim"

// CheckpointSource says which checkpoint each engine of a cluster is made from when the cluster
// does not start at tick 0: a replay or a recovery starts an engine from a checkpoint of its own
// and runs it on its log. The builder of the cluster asks it for each engine it makes.
type CheckpointSource interface {
	// Checkpoint returns the bytes of the checkpoint (EngineCheckpoint.Encode) that the engine is
	// made from, and false when it has none: the engine is made new, at the tick of its state. An
	// error is one that the checkpoint could not be read.
	Checkpoint(engine sim.StateID) ([]byte, bool, error)
}

// NoCheckpoints is the CheckpointSource of a cluster in which every engine starts new.
type NoCheckpoints struct{}

// Checkpoint says that no engine has one.
func (NoCheckpoints) Checkpoint(sim.StateID) ([]byte, bool, error) { return nil, false, nil }
