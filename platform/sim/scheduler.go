package sim

import (
	"fmt"
	"slices"

	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// Scheduler keeps what an engine runs at a later tick: the actions that were set for one tick,
// the next one included. What must happen again and again is an action that sets itself again when
// it runs (R7), so there is nothing else for the scheduler to keep: all that the engine will do on
// its own waits here. The tick count it knows is the last one it gave out, so an action set for a
// tick that has passed runs at the next one.
type Scheduler[S State] struct {
	at   map[uint64][]Action[S]
	last uint64
}

// At sets an action to run at the tick. A tick that is not in the future is the next one.
func (s *Scheduler[S]) At(tick uint64, a Action[S]) {
	if tick <= s.last {
		tick = s.last + 1
	}
	if s.at == nil {
		s.at = make(map[uint64][]Action[S])
	}
	s.at[tick] = append(s.at[tick], a)
}

// Due returns what runs at the tick, in the order it was set. Ticks are asked for in rising order.
func (s *Scheduler[S]) Due(tick uint64) []Action[S] {
	s.last = tick
	out := s.at[tick]
	delete(s.at, tick)
	return out
}

// Pending returns how many actions are set for a tick that did not come yet.
func (s *Scheduler[S]) Pending() int {
	n := 0
	for _, as := range s.at {
		n += len(as)
	}
	return n
}

// FindByID returns the action of the ID that waits for the earliest tick, the first of that tick
// when there are more.
func (s *Scheduler[S]) FindByID(id ActionID) (Action[S], bool) {
	var found Action[S]
	var foundAt uint64
	ok := false
	for tick, actions := range s.at {
		for _, a := range actions {
			if a.ID() == id && (!ok || tick < foundAt) {
				found, foundAt, ok = a, tick, true
				break // the first of this tick is the earliest of it
			}
		}
	}
	return found, ok
}

// Encode writes what the scheduler waits for: the last tick it gave out, and the actions
// set for a later tick, by tick and in the order they were set. An action that sets itself again
// is among them, so what the engine does again and again is written with the rest.
func (s *Scheduler[S]) Encode(w *binary.Writer) error {
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

// Decode reads what Encode wrote, of the data version given, and makes it what
// the scheduler waits for.
func (s *Scheduler[S]) Decode(
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
	at := make(map[uint64][]Action[S], n)
	for range n {
		tick := r.Uvarint()
		if _, dup := at[tick]; dup {
			return fmt.Errorf("%w: the tick %d is twice", binary.ErrBadValue, tick)
		}
		actions, err := c.DecodeActions[S](r, version)
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

// Hash adds what the scheduler waits for to the hash: the last tick, and the actions of
// each tick that has any, in the order of the ticks.
func (s *Scheduler[S]) Hash(h *Hasher) {
	h.U64(s.last)
	ticks := make([]uint64, 0, len(s.at))
	for tick, actions := range s.at {
		if len(actions) > 0 {
			ticks = append(ticks, tick)
		}
	}
	slices.Sort(ticks)
	var w binary.Writer
	for _, tick := range ticks {
		h.U64(tick)
		h.actionsWith(s.at[tick], &w)
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

var _ ReadOnlyScheduler[State] = (*Scheduler[State])(nil)
