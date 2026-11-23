package engine

import (
	"fmt"
	"math/rand/v2"
	"slices"

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

// engineHash is the hash of an engine: of everything a checkpoint of it holds that the engine goes
// on from: its state, what it waits for (the scheduler), and what only it knows (the place of its
// random numbers, the number of its messages and the last it took from each sender, the version of
// the configuration it read, and the messages it holds to send again). A restore that differs
// from the engine it came from in any of them has another hash, and a replay finds it.
func (c *cell) engineHash[S sim.State](
	stateHash uint64, sched *sim.Scheduler[S],
) uint64 {
	h := sim.NewHasher()
	h.U64(stateHash)
	sched.Hash(&h)
	c.hashInto(&h)
	return h.Sum()
}

// hashInto adds what every engine knows besides its state to a hash, as the bytes that a
// checkpoint writes for it, so that the hash covers exactly what the checkpoint carries.
func (c *cell) hashInto(h *sim.Hasher) {
	var w binary.Writer
	if err := c.encode(&w); err != nil {
		h.String("a cell that cannot be written") // the hash still tells it from another
		return
	}
	h.Bytes(w.B)
}

// checkpoint writes the checkpoint of an engine, given its state.
func (c *cell) checkpoint[S sim.State](
	tick uint64, st S, sched *sim.Scheduler[S],
) ([]byte, error) {
	var w binary.Writer
	w.Uvarint(1) // the version of the data
	w.Uvarint(uint64(c.me))
	w.Uvarint(tick)
	if err := st.Encode(&w); err != nil {
		return nil, fmt.Errorf("%v: the state: %w", c.me, err)
	}
	if err := sched.Encode(&w); err != nil {
		return nil, fmt.Errorf("%v: the scheduler: %w", c.me, err)
	}
	if err := c.encode(&w); err != nil {
		return nil, fmt.Errorf("%v: %w", c.me, err)
	}
	return w.B, nil
}

// readCheckpointStart reads the start of a checkpoint and checks that it is the checkpoint of an
// engine whose state is of the type.
func readCheckpointStart(
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

// restored is what follows the state of an engine in a checkpoint, read and checked but not yet
// given to an engine: nothing of an engine is made, and so nothing is registered in a router, until
// all of the checkpoint has been read.
type restored[S sim.State] struct {
	sched sim.Scheduler[S]
	cell  restoredCell
}

// restoredCell is what a checkpoint says of what every engine knows besides its state.
type restoredCell struct {
	pcg            *rand.PCG
	seq            uint64
	cfgVersionSeen uint64
	lastSeq        map[sim.StateID]uint64
	pending        []sim.Message
}

// readCheckpointRest reads what follows the state of an engine: what it waits for, and what only it
// knows.
func (env *Env) readCheckpointRest[S sim.State](
	r *binary.Reader, version codec.Version,
) (*restored[S], error) {
	var rest restored[S]
	if err := rest.sched.Decode(env.Codecs, r, version); err != nil {
		return nil, fmt.Errorf("the scheduler: %w", err)
	}
	var err error
	if rest.cell, err = decodeCell(env.Codecs, r, version); err != nil {
		return nil, err
	}
	if r.Left() != 0 {
		return nil, fmt.Errorf("%d bytes are left after the checkpoint", r.Left())
	}
	return &rest, nil
}

// encode writes what every engine knows besides its state: the place of its random numbers, the
// number of its messages and the last that it took from each sender, the version of the
// configuration it read, and the messages that it holds to send again.
func (c *cell) encode(w *binary.Writer) error {
	random, err := c.pcg.MarshalBinary()
	if err != nil {
		return fmt.Errorf("its random numbers: %w", err)
	}
	w.String(string(random))
	w.Uvarint(c.seq)
	w.Uvarint(c.cfgVersionSeen)
	senders := make([]sim.StateID, 0, len(c.lastSeq))
	for sender := range c.lastSeq {
		senders = append(senders, sender)
	}
	slices.Sort(senders)
	w.Uvarint(uint64(len(senders)))
	for _, sender := range senders {
		w.Uvarint(uint64(sender))
		w.Uvarint(c.lastSeq[sender])
	}
	w.Uvarint(uint64(len(c.bounce.pending)))
	for _, m := range c.bounce.pending {
		if err := sim.EncodeMessage(w, m); err != nil {
			return fmt.Errorf("a message it holds to send again: %w", err)
		}
	}
	return nil
}

// decodeCell reads what encode wrote.
func decodeCell(
	c *sim.Codecs, r *binary.Reader, version codec.Version,
) (restoredCell, error) {
	rc := restoredCell{pcg: rand.NewPCG(0, 0)}
	if err := rc.pcg.UnmarshalBinary([]byte(r.String())); err != nil {
		return restoredCell{}, fmt.Errorf("its random numbers: %w", err)
	}
	rc.seq = r.Uvarint()
	rc.cfgVersionSeen = r.Uvarint()
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return restoredCell{}, err
	}
	if n > uint64(r.Left()) { // a sender takes at least one byte
		return restoredCell{}, fmt.Errorf("%w: %d senders", binary.ErrShort, n)
	}
	rc.lastSeq = make(map[sim.StateID]uint64, n)
	for range n {
		sender := r.Uint[sim.StateID]()
		rc.lastSeq[sender] = r.Uvarint()
	}
	n = r.Uvarint()
	if err := r.Err(); err != nil {
		return restoredCell{}, err
	}
	if n > uint64(r.Left()) { // a message takes at least one byte
		return restoredCell{}, fmt.Errorf("%w: %d messages", binary.ErrShort, n)
	}
	for range n {
		m, err := c.DecodeMessage(r, version)
		if err != nil {
			return restoredCell{}, fmt.Errorf("a message it holds to send again: %w", err)
		}
		rc.pending = append(rc.pending, m)
	}
	return rc, r.Err()
}

// restore gives the cell what a checkpoint says. It cannot fail: all of it was read and checked
// before the engine was made.
func (c *cell) restore(rc restoredCell) {
	*c.pcg = *rc.pcg
	c.seq, c.cfgVersionSeen = rc.seq, rc.cfgVersionSeen
	c.lastSeq = rc.lastSeq
	c.bounce.pending = rc.pending
}
