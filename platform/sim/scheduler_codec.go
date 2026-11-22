package sim

import (
	"fmt"
	"slices"

	"github.com/huykingsofm/project-b/shared/binarycodec"
)

// EncodeActions writes a list of actions: how many, and each. It returns an error, and the writer
// holds a partial list, when an action cannot be written.
func EncodeActions[S State](w *binarycodec.Writer, actions []Action[S]) error {
	w.Uvarint(uint64(len(actions)))
	for _, a := range actions {
		if err := EncodeAction(w, a); err != nil {
			return err
		}
	}
	return nil
}

// HashActions adds a list of actions to the hash: each as the bytes it is written as, which say
// everything it holds, the parts that only it knows included. An action that cannot be written adds
// its type, so the hash still tells it from another.
func HashActions[S State](h *Hasher, actions []Action[S]) {
	var w binarycodec.Writer
	hashActions(h, actions, &w)
}

// hashActions is HashActions with the bytes to write each action to, which a caller that hashes
// many lists makes once.
func hashActions[S State](h *Hasher, actions []Action[S], w *binarycodec.Writer) {
	h.U64(uint64(len(actions)))
	for _, a := range actions {
		w.B = w.B[:0]
		if EncodeAction(w, a) != nil {
			h.String(fmt.Sprintf("%T", a))
			continue
		}
		h.Bytes(w.B)
	}
}

// EncodeScheduler writes what the scheduler waits for: the last tick it gave out, and the actions
// set for a later tick, by tick and in the order they were set. An action that sets itself again
// is among them, so what the engine does again and again is written with the rest.
func EncodeScheduler[S State](w *binarycodec.Writer, s *Scheduler[S]) error {
	w.Uvarint(s.last)
	ticks := make([]uint64, 0, len(s.at))
	for tick, actions := range s.at {
		if len(actions) > 0 {
			ticks = append(ticks, tick)
		}
	}
	slices.Sort(ticks)
	w.Uvarint(uint64(len(ticks)))
	for _, tick := range ticks {
		w.Uvarint(tick)
		if err := EncodeActions(w, s.at[tick]); err != nil {
			return err
		}
	}
	return nil
}

// DecodeScheduler reads what EncodeScheduler wrote, of the data version given, and makes it what
// the scheduler waits for.
func DecodeScheduler[S State](
	c *Codecs, r *binarycodec.Reader, version binarycodec.Version, s *Scheduler[S],
) error {
	last := r.Uvarint()
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return err
	}
	if n > uint64(r.Left()) { // a tick takes at least one byte
		return fmt.Errorf("%w: %d ticks", binarycodec.ErrShort, n)
	}
	at := make(map[uint64][]Action[S], n)
	for range n {
		tick := r.Uvarint()
		actions, err := DecodeActions[S](c, r, version)
		if err != nil {
			return err
		}
		at[tick] = actions
	}
	if err := r.Err(); err != nil {
		return err
	}
	s.last, s.at = last, at
	return nil
}

// HashScheduler adds what the scheduler waits for to the hash: the last tick, and the actions of
// each tick that has any, in the order of the ticks.
func HashScheduler[S State](h *Hasher, s *Scheduler[S]) {
	h.U64(s.last)
	ticks := make([]uint64, 0, len(s.at))
	for tick, actions := range s.at {
		if len(actions) > 0 {
			ticks = append(ticks, tick)
		}
	}
	slices.Sort(ticks)
	var w binarycodec.Writer
	for _, tick := range ticks {
		h.U64(tick)
		hashActions(h, s.at[tick], &w)
	}
}
