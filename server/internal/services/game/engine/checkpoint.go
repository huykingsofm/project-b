package engine

import (
	"fmt"
	"slices"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// A checkpoint is what an engine needs to go on from the tick it was taken at, as bytes: the start
// says which engine it is (the version of the data, the address, the tick), then its state, then
// what the engine waits for and what only the engine knows.
//
// It is taken between two ticks, by the goroutine of the engine or while the engine does not run.
// The queues of the engine are not in it: what was not yet taken from them is in the replay log.
// What the engine does again and again is an action that waits, so it is in the scheduler and
// there is nothing else to write for it.

// engineHash is the hash of an engine: of everything a checkpoint of it holds that the engine goes
// on from: its state, what it waits for (the scheduler and the actions it set for itself for the
// next tick), and what only it knows (the place of its random numbers, the number of its messages
// and the last it took from each sender, the version of the configuration it read, and the
// messages it holds to send again). A restore that differs from the engine it came from in any of
// them has another hash, and a replay finds it.
func engineHash[A action.Action](
	c *cell, stateHash uint64, sched *action.Scheduler[A], self []A,
) uint64 {
	h := state.NewHasher()
	h.U64(stateHash)
	action.HashScheduler(&h, sched)
	action.HashActions(&h, self)
	c.hashInto(&h)
	return h.Sum()
}

// hashInto adds what every engine knows besides its state to a hash, as the bytes that a
// checkpoint writes for it, so that the hash covers exactly what the checkpoint carries.
func (c *cell) hashInto(h *state.Hasher) {
	var w binarycodec.Writer
	if err := c.encode(&w); err != nil {
		h.String("a cell that cannot be written") // the hash still tells it from another
		return
	}
	h.Bytes(w.B)
}

// checkpoint writes the checkpoint of an engine of a scope, given how its state is written.
func checkpoint[A action.Action](
	c *cell,
	tick uint64,
	encodeState func(w *binarycodec.Writer) error,
	sched *action.Scheduler[A],
	self []A,
) ([]byte, error) {
	var w binarycodec.Writer
	w.Uvarint(1) // the version of the data
	c.me.Encode(&w)
	w.Uvarint(tick)
	if err := encodeState(&w); err != nil {
		return nil, fmt.Errorf("%v: the state: %w", c.me, err)
	}
	if err := action.EncodeScheduler(&w, sched); err != nil {
		return nil, fmt.Errorf("%v: the scheduler: %w", c.me, err)
	}
	if err := action.EncodeActions(&w, self); err != nil {
		return nil, fmt.Errorf("%v: what it set for itself: %w", c.me, err)
	}
	if err := c.encode(&w); err != nil {
		return nil, fmt.Errorf("%v: %w", c.me, err)
	}
	return w.B, nil
}

// readCheckpointStart reads the start of a checkpoint and checks that it is the checkpoint of an
// engine of the scope.
func readCheckpointStart(
	r *binarycodec.Reader, scope message.Scope,
) (binarycodec.Version, message.Address, uint64, error) {
	version := binarycodec.Version(r.Uvarint())
	if err := r.Err(); err != nil {
		return 0, message.Address{}, 0, err
	}
	if version != 1 {
		return 0, message.Address{}, 0, fmt.Errorf(
			"the checkpoint is of the data version %d", version,
		)
	}
	address := message.DecodeAddress(r, version)
	tick := r.Uvarint()
	if err := r.Err(); err != nil {
		return 0, message.Address{}, 0, err
	}
	if address.Scope != scope {
		return 0, message.Address{}, 0, fmt.Errorf(
			"the checkpoint is of %v, not of an engine of a %v", address, scope,
		)
	}
	return version, address, tick, nil
}

// readCheckpointRest reads what follows the state of an engine, and gives the engine what it
// waits for, what it set for itself and what only it knows. It returns what it set for itself.
func readCheckpointRest[A action.Action](
	r *binarycodec.Reader, version binarycodec.Version, c *cell, sched *action.Scheduler[A],
) ([]A, error) {
	if err := action.DecodeScheduler(r, version, sched); err != nil {
		return nil, fmt.Errorf("the scheduler: %w", err)
	}
	self, err := action.DecodeActions[A](r, version)
	if err != nil {
		return nil, fmt.Errorf("what it set for itself: %w", err)
	}
	if err := c.decode(r, version); err != nil {
		return nil, err
	}
	if r.Left() != 0 {
		return nil, fmt.Errorf("%d bytes are left after the checkpoint", r.Left())
	}
	return self, nil
}

// encode writes what every engine knows besides its state: the place of its random numbers, the
// number of its messages and the last that it took from each sender, the version of the
// configuration it read, and the messages that it holds to send again.
func (c *cell) encode(w *binarycodec.Writer) error {
	random, err := c.pcg.MarshalBinary()
	if err != nil {
		return fmt.Errorf("its random numbers: %w", err)
	}
	w.String(string(random))
	w.Uvarint(c.seq)
	w.Uvarint(c.cfgVersionSeen)
	senders := make([]message.Address, 0, len(c.lastSeq))
	for sender := range c.lastSeq {
		senders = append(senders, sender)
	}
	slices.SortFunc(senders, func(a, b message.Address) int {
		if a.Scope != b.Scope {
			return int(a.Scope) - int(b.Scope)
		}
		return int(a.ID) - int(b.ID)
	})
	w.Uvarint(uint64(len(senders)))
	for _, sender := range senders {
		sender.Encode(w)
		w.Uvarint(c.lastSeq[sender])
	}
	w.Uvarint(uint64(len(c.bounce.pending)))
	for _, m := range c.bounce.pending {
		if err := action.EncodeMessage(w, m); err != nil {
			return fmt.Errorf("a message it holds to send again: %w", err)
		}
	}
	return nil
}

// decode reads what encode wrote.
func (c *cell) decode(r *binarycodec.Reader, version binarycodec.Version) error {
	if err := c.pcg.UnmarshalBinary([]byte(r.String())); err != nil {
		return fmt.Errorf("its random numbers: %w", err)
	}
	c.seq = r.Uvarint()
	c.cfgVersionSeen = r.Uvarint()
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return err
	}
	if n > uint64(r.Left()) { // a sender takes at least one byte
		return fmt.Errorf("%w: %d senders", binarycodec.ErrShort, n)
	}
	c.lastSeq = make(map[message.Address]uint64, n)
	for range n {
		sender := message.DecodeAddress(r, version)
		c.lastSeq[sender] = r.Uvarint()
	}
	n = r.Uvarint()
	if err := r.Err(); err != nil {
		return err
	}
	if n > uint64(r.Left()) { // a message takes at least one byte
		return fmt.Errorf("%w: %d messages", binarycodec.ErrShort, n)
	}
	c.bounce.pending = nil
	for range n {
		m, err := action.DecodeMessage(r, version)
		if err != nil {
			return fmt.Errorf("a message it holds to send again: %w", err)
		}
		c.bounce.pending = append(c.bounce.pending, m)
	}
	return r.Err()
}
