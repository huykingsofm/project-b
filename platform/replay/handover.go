package replay

import (
	"errors"
	"fmt"

	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/sim"
)

// encodedCheckpoints is the CheckpointSource of the engines that are made again from the replay of
// the engines they were: the bytes of each checkpoint, by engine.
type encodedCheckpoints map[sim.StateID][]byte

func (c encodedCheckpoints) Checkpoint(id sim.StateID) ([]byte, bool, error) {
	data, ok := c[id]
	return data, ok, nil
}

// Handover makes the engines of the cluster again in the environment given, each from a checkpoint
// of its replayed engine at its limit, and returns the cluster, which has not run yet: the caller
// runs it, and the engines go on from their own ticks. The environment is the one of a cluster
// that runs (its router, event log, published states, emitter, acks and checkpoint writer); the
// event log in it writes on after the cut, so no log may be open for writing before.
//
// Each engine is given the messages that it sent in the replay as its sent messages that are not
// acked, so at its first flush it sends them again: a receiver that took one has it in its log,
// drops the second copy by its number and acks it, and one that did not take it takes it now.
// Each engine publishes its state before its first tick, for the engines that read it.
//
// A replay in which an engine differs from its log is not handed over. A checkpoint that cannot
// make its engine, or an engine that the builder makes and the replay does not have, is an error.
func (r *Result) Handover(b Builder, env *engine.Env) (*engine.Cluster, error) {
	if len(r.Divergences) > 0 {
		return nil, errors.New("handover: the replay differs from its logs")
	}
	checkpoints := encodedCheckpoints{}
	for _, e := range r.Cluster.Engines() {
		data, err := e.EncodeCheckpoint()
		if err != nil {
			return nil, fmt.Errorf("handover: the checkpoint of %v: %w", e.StateID(), err)
		}
		checkpoints[e.StateID()] = data
	}
	live, err := b.BuildIn(env, checkpoints)
	if err != nil {
		return nil, fmt.Errorf("handover: %w", err)
	}
	for _, e := range live.Engines() {
		id := e.StateID()
		if _, ok := checkpoints[id]; !ok {
			return nil, fmt.Errorf("handover: the engine %v was not in the replay", id)
		}
		e.SetSentMessages(r.SentMessages[id])
		e.PublishState()
	}
	return live, nil
}
