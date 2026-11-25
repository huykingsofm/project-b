package engine

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
)

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

// NewOrRestore makes the engine of a state that was just made: from the checkpoint that the source
// has for its ID, if it has one, whatever the state says (it gives the engine its ID and nothing
// else), and new otherwise. It returns an error when the checkpoint cannot be read or cannot make
// its engine.
func NewOrRestore[S sim.State](
	fresh S, env *Env, seed uint64, checkpoints CheckpointSource,
) (*Engine[S], error) {
	data, ok, err := checkpoints.Checkpoint(fresh.ID())
	if err != nil {
		return nil, fmt.Errorf("the checkpoint of %v: %w", fresh.ID(), err)
	}
	if !ok {
		return New(fresh, env, seed), nil
	}
	c, err := DecodeEngineCheckpoint[S](data, env.Codecs)
	if err != nil {
		return nil, fmt.Errorf("the checkpoint of %v: %w", fresh.ID(), err)
	}
	return c.Restore(env, seed)
}
