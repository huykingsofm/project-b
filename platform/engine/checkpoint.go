package engine

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// A checkpoint is what an engine needs to go on from the tick it was taken at, as bytes: the start
// says which engine it is (the version of the data, its state ID, which has the type of the
// state in it, the tick), then its state, then what the engine waits for and what only the engine
// knows.
//
// It is taken between two ticks, by the goroutine of the engine or while the engine does not run.
// The queues of the engine are not in it: what was not yet taken from them is in the replay log.
// What the engine does again and again is an action that waits, so it is in the scheduler and
// there is nothing else to write for it.

// CheckpointEngine writes the checkpoint of an engine: the start, which says which engine it is and
// the tick it is at, then its state, what it waits for (the scheduler), and what only it knows. It
// is called between two ticks, by the goroutine of the engine or while the engine does not run.
func CheckpointEngine[S sim.State](e *Engine[S]) ([]byte, error) {
	var w binary.Writer
	w.Uvarint(1) // the version of the data
	w.Uvarint(uint64(e.me))
	w.Uvarint(e.tick)
	if err := e.st.Encode(&w); err != nil {
		return nil, fmt.Errorf("%v: the state: %w", e.me, err)
	}
	if err := e.sched.Encode(&w); err != nil {
		return nil, fmt.Errorf("%v: the scheduler: %w", e.me, err)
	}
	if err := e.cell.cellData.Encode(&w); err != nil {
		return nil, fmt.Errorf("%v: %w", e.me, err)
	}
	return w.B, nil
}

// RestoreEngine makes the engine of a state from a checkpoint, in the environment given, at the
// tick of the checkpoint. The state is read with the codecs of the environment. What the engine
// does again and again is an action that waits in the checkpoint like any other. Nothing of an
// engine is made, and so nothing is registered in a router, until all of the checkpoint has been
// read and checked: a bad checkpoint returns an error and registers nothing.
func RestoreEngine[S sim.State](data []byte, env *Env, seed uint64) (*Engine[S], error) {
	r := binary.NewReader(data)
	version, id, tick, err := readCheckpointHeader(r, sim.StateTypeOf[S]())
	if err != nil {
		return nil, err
	}
	decodedState, err := env.Codecs.DecodeStateOfType(sim.StateTypeOf[S](), r, version)
	if err != nil {
		return nil, fmt.Errorf("%d: the state: %w", id, err)
	}
	state, ok := decodedState.(S)
	if !ok {
		return nil, fmt.Errorf("%d: the state is a %T, not the one asked for", id, decodedState)
	}
	if state.ID() != id {
		return nil, fmt.Errorf("%d: the state is of the engine %d", id, state.ID())
	}
	if state.Tick() != tick {
		return nil, fmt.Errorf(
			"%d: %w: the state is at tick %d, and the checkpoint at %d",
			id, ErrTickMismatch, state.Tick(), tick,
		)
	}
	var sched sim.Scheduler[S]
	if err := sched.Decode(env.Codecs, r, version); err != nil {
		return nil, fmt.Errorf("%d: the scheduler: %w", id, err)
	}
	cellData, err := decodeCellData(env.Codecs, r, version)
	if err != nil {
		return nil, fmt.Errorf("%d: %w", id, err)
	}
	if r.Left() != 0 {
		return nil, fmt.Errorf("%d: %d bytes are left after the checkpoint", id, r.Left())
	}
	e := New(state, env, seed)
	e.tick = tick
	e.sched = sched
	e.cell.restoreFrom(cellData)
	return e, nil
}

// readCheckpointHeader reads the start of a checkpoint and checks that it is the checkpoint of an
// engine whose state is of the type.
func readCheckpointHeader(
	r *binary.Reader, want sim.StateType,
) (codec.Version, sim.StateID, uint64, error) {
	version := r.Uint[codec.Version]()
	if err := r.Err(); err != nil {
		return 0, 0, 0, err
	}
	if version != 1 {
		return 0, 0, 0, fmt.Errorf("the checkpoint is of the data version %d", version)
	}
	id := r.Uint[sim.StateID]()
	tick := r.Uvarint()
	if err := r.Err(); err != nil {
		return 0, 0, 0, err
	}
	if id.Type() != want {
		return 0, 0, 0, fmt.Errorf(
			"the checkpoint is of the engine %d, whose state is of the type %d, not %d",
			id, id.Type(), want,
		)
	}
	return version, id, tick, nil
}
