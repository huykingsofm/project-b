package sim

import (
	"fmt"
	"maps"
	"slices"

	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// StateIDCounters gives out the IDs of states, of any type: the local number of a type is counted
// up from 1 and is never given again, so a StateID names one state for good, in a log, a
// checkpoint or a message that waits. It is plain data that a state holds, writes to its
// checkpoint and works into its hash. Make one with NewStateIDCounters.
type StateIDCounters struct {
	last map[StateType]uint64 // the local number last given, for each type that was given one
	// ordered is the types that have a counter, from the smallest, for a fixed order
	ordered []StateType
}

// NewStateIDCounters makes a counter of state IDs that has given none.
func NewStateIDCounters() StateIDCounters {
	return StateIDCounters{last: map[StateType]uint64{}}
}

// Give returns the next state ID of the type. It reports false when the type has no number left:
// the 56 bits of a local number are all given.
func (s *StateIDCounters) Give(t StateType) (StateID, bool) {
	last, known := s.last[t]
	if last >= maxLocalStateID {
		return 0, false
	}
	if !known {
		i, _ := slices.BinarySearch(s.ordered, t)
		s.ordered = slices.Insert(s.ordered, i, t)
	}
	s.last[t] = last + 1
	return NewStateID(t, last+1), true
}

// Last is the local number that was last given for the type, or 0 when none was.
func (s *StateIDCounters) Last(t StateType) uint64 { return s.last[t] }

// Clone returns a copy that shares nothing.
func (s *StateIDCounters) Clone() StateIDCounters {
	c := StateIDCounters{
		last: make(map[StateType]uint64, len(s.last)), ordered: slices.Clone(s.ordered),
	}
	maps.Copy(c.last, s.last)
	return c
}

// Hash works what it holds into a hash: each type, from the smallest, and the number last given.
func (s *StateIDCounters) Hash(h *Hasher) {
	for _, t := range s.ordered {
		h.U64(uint64(t))
		h.U64(s.last[t])
	}
}

// Encode writes it: the number of types, and then each type, from the smallest, with the number
// last given.
func (s *StateIDCounters) Encode(w *binary.Writer) {
	w.Uvarint(uint64(len(s.ordered)))
	for _, t := range s.ordered {
		w.Uint8(uint8(t))
		w.Uvarint(s.last[t])
	}
}

// DecodeStateIDCounters reads what Encode wrote. It refuses a type that is there twice or out of
// order, and a number that is 0 or more than a local number holds.
func DecodeStateIDCounters(r *binary.Reader) (StateIDCounters, error) {
	s := NewStateIDCounters()
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return StateIDCounters{}, err
	}
	if n > uint64(r.Left()) { // a type takes at least two bytes
		return StateIDCounters{}, fmt.Errorf("%w: %d types", binary.ErrShort, n)
	}
	for range n {
		t := StateType(r.Uint8())
		last := r.Uvarint()
		if err := r.Err(); err != nil {
			return StateIDCounters{}, err
		}
		if len(s.ordered) > 0 && t <= s.ordered[len(s.ordered)-1] {
			return StateIDCounters{}, fmt.Errorf("the state IDs of the type %d are not in order", t)
		}
		if last == 0 || last > maxLocalStateID {
			return StateIDCounters{}, fmt.Errorf(
				"the state IDs of the type %d were given up to %d", t, last,
			)
		}
		s.last[t] = last
		s.ordered = append(s.ordered, t)
	}
	return s, r.Err()
}
