package sim

import (
	"fmt"

	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/stablemap"
)

// ActionScheduler keeps what an engine runs at a later tick: the actions that were set for one
// tick, the next one included. What must happen again and again is an action that sets itself
// again when it runs (R7), so there is nothing else for the scheduler to keep: all that the engine
// will do on its own waits here. The tick count it knows is the last one it gave out, so an action
// set for a tick that has passed runs at the next one.
//
// The ticks are kept in a stablemap: they are visited in an order that depends on the calls of At
// and Due and on nothing else, so two runs that are given the same calls write and hash the same
// bytes, and a scheduler read back from what it wrote visits them in the order it was written.
type ActionScheduler[S State] struct {
	at   stablemap.Map[uint64, []Action[S]]
	last uint64
}

// At sets an action to run at the tick. A tick that is not in the future is the next one.
func (s *ActionScheduler[S]) At(tick uint64, a Action[S]) {
	if tick <= s.last {
		tick = s.last + 1
	}
	if actions := s.at.Ptr(tick); actions != nil {
		*actions = append(*actions, a)
		return
	}
	s.at.Set(tick, []Action[S]{a})
}

// Due returns what runs at the tick, in the order it was set. Ticks are asked for in rising order.
func (s *ActionScheduler[S]) Due(tick uint64) []Action[S] {
	s.last = tick
	out, _ := s.at.Get(tick)
	s.at.Delete(tick)
	return out
}

// Pending returns how many actions are set for a tick that did not come yet.
func (s *ActionScheduler[S]) Pending() int {
	n := 0
	for _, actions := range s.at.All() {
		n += len(actions)
	}
	return n
}

// FindByID returns the action of the ID that waits for the earliest tick, the first of that tick
// when there are more.
func (s *ActionScheduler[S]) FindByID(id ActionID) (Action[S], bool) {
	var found Action[S]
	var foundAt uint64
	ok := false
	for tick, actions := range s.at.All() {
		for _, a := range actions {
			if a.ID() == id && (!ok || tick < foundAt) {
				found, foundAt, ok = a, tick, true
				break // the first of this tick is the earliest of it
			}
		}
	}
	return found, ok
}

// Clone returns a copy that shares nothing with the scheduler: each action is copied with its
// Clone.
func (s *ActionScheduler[S]) Clone() ActionScheduler[S] {
	return ActionScheduler[S]{
		at: s.at.CloneFunc(func(actions []Action[S]) []Action[S] {
			out := make([]Action[S], len(actions))
			for i, a := range actions {
				out[i] = a.Clone()
			}
			return out
		}),
		last: s.last,
	}
}

// Encode writes what the scheduler waits for: the last tick it gave out, and the actions set for a
// later tick, by tick in the order the scheduler keeps them and each tick in the order its actions
// were set. An action that sets itself again is among them, so what the engine does again and
// again is written with the rest.
func (s *ActionScheduler[S]) Encode(w *binary.Writer) error {
	w.Uvarint(s.last)
	w.Uvarint(uint64(s.at.Len()))
	for tick, actions := range s.at.All() {
		w.Uvarint(tick)
		if err := EncodeActions(w, actions); err != nil {
			return err
		}
	}
	return nil
}

// Decode reads what Encode wrote, of the data version given, and makes it what the scheduler waits
// for, in the order it was written.
func (s *ActionScheduler[S]) Decode(
	c *Codecs, r *binary.Reader, version codec.Version,
) error {
	last := r.Uvarint()
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return err
	}
	if n > uint64(r.Left()) { // a tick takes at least one byte
		return fmt.Errorf("%w: %d ticks", binary.ErrShort, n)
	}
	var at stablemap.Map[uint64, []Action[S]]
	for range n {
		tick := r.Uvarint()
		if at.Has(tick) {
			return fmt.Errorf("%w: the tick %d is twice", binary.ErrBadValue, tick)
		}
		actions, err := c.DecodeActions[S](r, version)
		if err != nil {
			return err
		}
		at.Set(tick, actions)
	}
	if err := r.Err(); err != nil {
		return err
	}
	s.last, s.at = last, at
	return nil
}

// Hash adds what the scheduler waits for to the hash: the last tick, and the actions of each tick,
// in the order the scheduler keeps them.
func (s *ActionScheduler[S]) Hash(h *Hasher) {
	h.U64(s.last)
	var w binary.Writer
	for tick, actions := range s.at.All() {
		h.U64(tick)
		h.actionsWith(actions, &w)
	}
}

// ReadOnlyScheduler is what the outside of an engine may ask of its scheduler: what waits in it.
// Only the engine changes the scheduler, at the end of its tick.
type ReadOnlyScheduler[S State] interface {
	// FindByID returns the action of the ID that waits for the earliest tick.
	FindByID(id ActionID) (Action[S], bool)
	// Pending returns how many actions wait for a tick.
	Pending() int
}

var _ ReadOnlyScheduler[State] = (*ActionScheduler[State])(nil)
