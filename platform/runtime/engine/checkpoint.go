package engine

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/rand"
)

// CheckpointWriter writes, out of the goroutines of the engines, the checkpoints that they take at
// the end of a tick. Offer does not wait: it is called by the goroutine of the engine, and a
// checkpoint that it cannot take now is dropped, which the writer says itself. encode makes the
// bytes of the checkpoint (Checkpoint.Encode) and is called once, on the goroutine of the
// writer.
type CheckpointWriter interface {
	Offer(engine sim.StateID, tick uint64, encode func() ([]byte, error))
}

// Checkpoint is what an engine of a state of type S needs to go on from the tick it was taken
// at: its state, what it waits for, what only it knows and what its peer knows. It shares nothing
// with the engine it was taken from, which runs on, so it may be encoded on another goroutine.
//
// The queues of the engine are not in it: what was not yet taken from them is in the event log.
// What the engine does again and again is an action that waits, so it is in the scheduler and
// there is nothing else to keep for it.
type Checkpoint[S sim.State] struct {
	ID              sim.StateID
	Tick            uint64
	State           S
	ActionScheduler sim.ActionScheduler[S]
	Rand            *rand.Rand
	Peer            peerData
}

// DecodeCheckpoint reads a checkpoint that Encode wrote, of an engine whose state is of type
// S, with the codecs given, which for a zone know the map of the zone. A checkpoint that is cut,
// has a byte too many, is of another data version, or of an engine whose state is of another type
// than S is an error.
func DecodeCheckpoint[S sim.State](
	data []byte, codecs *sim.Codecs,
) (Checkpoint[S], error) {
	r := binary.NewReader(data)
	version := r.Uint[codec.Version]()
	id := r.Uint[sim.StateID]()
	tick := r.Uvarint()
	if err := r.Err(); err != nil {
		return Checkpoint[S]{}, err
	}
	if version != 1 {
		return Checkpoint[S]{}, fmt.Errorf(
			"the checkpoint is of the data version %d", version,
		)
	}
	if want := sim.StateTypeOf[S](); id.Type() != want {
		return Checkpoint[S]{}, fmt.Errorf(
			"the checkpoint is of the engine %d, whose state is of the type %d, not %d",
			id, id.Type(), want,
		)
	}
	c := Checkpoint[S]{ID: id, Tick: tick}
	decoded, err := codecs.DecodeStateOfType(sim.StateTypeOf[S](), r, version)
	if err != nil {
		return Checkpoint[S]{}, fmt.Errorf("%d: the state: %w", id, err)
	}
	state, ok := decoded.(S)
	if !ok {
		return Checkpoint[S]{}, fmt.Errorf(
			"%d: the state is a %T, not the one asked for", id, decoded,
		)
	}
	if state.Tick() != tick {
		return Checkpoint[S]{}, fmt.Errorf(
			"%d: %w: the state is at tick %d, and the checkpoint at %d",
			id, ErrTickMismatch, state.Tick(), tick,
		)
	}
	c.State = state
	if err := c.ActionScheduler.Decode(codecs, r, version); err != nil {
		return Checkpoint[S]{}, fmt.Errorf("%d: the scheduler: %w", id, err)
	}
	if c.Rand, err = rand.Decode(r); err != nil {
		return Checkpoint[S]{}, fmt.Errorf("%d: its random numbers: %w", id, err)
	}
	if c.Peer, err = decodePeerData(codecs, r, version); err != nil {
		return Checkpoint[S]{}, fmt.Errorf("%d: %w", id, err)
	}
	if r.Left() != 0 {
		return Checkpoint[S]{}, fmt.Errorf(
			"%d: %d bytes are left after the checkpoint", id, r.Left(),
		)
	}
	return c, nil
}

// Encode writes the checkpoint: the version of the data, the state ID (which has the type of the
// state in it), the tick, then the state, the scheduler, the random numbers, and what the peer
// knows.
func (c Checkpoint[S]) Encode() ([]byte, error) {
	var w binary.Writer
	w.Uvarint(1) // the version of the data
	w.Uvarint(uint64(c.ID))
	w.Uvarint(c.Tick)
	if err := c.State.Encode(&w); err != nil {
		return nil, fmt.Errorf("%v: the state: %w", c.ID, err)
	}
	if err := c.ActionScheduler.Encode(&w); err != nil {
		return nil, fmt.Errorf("%v: the scheduler: %w", c.ID, err)
	}
	if err := c.Rand.Encode(&w); err != nil {
		return nil, fmt.Errorf("%v: its random numbers: %w", c.ID, err)
	}
	if err := c.Peer.Encode(&w); err != nil {
		return nil, fmt.Errorf("%v: %w", c.ID, err)
	}
	return w.B, nil
}

// Restore makes the engine of the checkpoint, in the environment given, at the tick of the
// checkpoint, and registers it in the router of the environment as New does. The engine takes what
// the checkpoint holds: the checkpoint is not used after. One whose state is not at its tick is an
// error, and registers nothing.
func (c Checkpoint[S]) Restore(env *Environment) (*Engine[S], error) {
	if c.State.Tick() != c.Tick {
		return nil, fmt.Errorf(
			"%d: %w: the state is at tick %d, and the checkpoint at %d",
			c.ID, ErrTickMismatch, c.State.Tick(), c.Tick,
		)
	}
	e := NewEngine(c.State, c.ID, env)
	e.tick = c.Tick
	e.actionScheduler = c.ActionScheduler
	e.rng = c.Rand
	e.peer.restoreFrom(c.Peer)
	return e, nil
}
