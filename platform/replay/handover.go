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
// The replay has an AckBoard, so each replayed engine holds, and its checkpoint carries, the
// messages it sent that no receiver took in the part of its log that was kept. At its first flush
// a live engine lets go of what its receivers acked from their checkpoints and sends the rest
// again: a receiver whose log was cut takes one now, and one that took it drops the copy by its
// number. Each engine that ran a tick publishes its state before its next tick, for the engines
// that read it.
//
// An engine that the replay removed (its log ends with the record of its removal) is not made
// again: the builder makes it, new, and it is removed at once, without writing to its log.
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
	// the builder makes the engines that were removed too, new: they are not the engines that were
	// recorded, whose logs end with the record of their removal
	for _, id := range r.Retired {
		if !live.RemoveAlreadyRecorded(id) {
			return nil, fmt.Errorf("handover: the engine %v was removed, the builder has none", id)
		}
	}
	for _, e := range live.Engines() {
		id := e.StateID()
		if _, ok := checkpoints[id]; !ok {
			return nil, fmt.Errorf("handover: the engine %v was not in the replay", id)
		}
		// an engine that has run no tick publishes nothing, as one that was never stopped does: a
		// read that finds no copy is recorded as the tick 0, and a copy at the tick 0 would be
		// recorded the same
		if e.Tick() > 0 {
			e.PublishState()
		}
	}
	return live, nil
}
