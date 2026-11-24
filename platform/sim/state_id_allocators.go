package sim

import (
	"fmt"
	"slices"

	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/ids"
)

// StateIDAllocators gives out the IDs of states, of any type: the local number of each type comes
// from an allocator of its own (package ids), from 1 up to what the 24 bits of a local number hold,
// with a grave of the same steps for every type. An ID that is given back waits in the grave before
// it is given again, so that what still names the state it was (a message that waits to be sent to
// it) has the time to let go. It is plain data that a state holds, writes to its checkpoint and
// works into its hash. Make one with NewStateIDAllocators.
type StateIDAllocators struct {
	steps   uint64
	byType  map[StateType]*ids.Allocator[uint32]
	ordered []StateType // the types that have an allocator, from the smallest, for a fixed order
}

// stateLocalLimit is the largest local number of a state ID.
const stateLocalLimit = 1<<24 - 1

// NewStateIDAllocators makes an allocator of state IDs whose grave keeps an ID for graveSteps
// steps.
func NewStateIDAllocators(graveSteps uint64) StateIDAllocators {
	return StateIDAllocators{steps: graveSteps, byType: map[StateType]*ids.Allocator[uint32]{}}
}

// GraveSteps is how many steps an ID waits in the grave.
func (s *StateIDAllocators) GraveSteps() uint64 { return s.steps }

// allocator returns the allocator of a type, making it the first time.
func (s *StateIDAllocators) allocator(t StateType) *ids.Allocator[uint32] {
	if a, ok := s.byType[t]; ok {
		return a
	}
	a := ids.New[uint32](s.steps, stateLocalLimit)
	s.byType[t] = &a
	i, _ := slices.BinarySearch(s.ordered, t)
	s.ordered = slices.Insert(s.ordered, i, t)
	return &a
}

// Give returns a state ID of the type that is live from now on. It reports false when the type has
// no number left, every one being live or in the grave.
func (s *StateIDAllocators) Give(t StateType) (StateID, bool) {
	local, ok := s.allocator(t).Give()
	if !ok {
		return 0, false
	}
	return NewStateID(t, local), true
}

// Release gives a live ID back: it waits in the grave for the steps. It reports false for an ID
// that is not live, which changes nothing.
func (s *StateIDAllocators) Release(id StateID) bool {
	a, ok := s.byType[id.Type()]
	return ok && a.Release(id.Local())
}

// Step takes every ID in the grave one step, of every type.
func (s *StateIDAllocators) Step() {
	for _, t := range s.ordered {
		s.byType[t].Step()
	}
}

// Status says what an ID is: live, in the grave, or not given.
func (s *StateIDAllocators) Status(id StateID) ids.Status {
	a, ok := s.byType[id.Type()]
	if !ok {
		return ids.NotGiven
	}
	return a.Status(id.Local())
}

// IsLive reports whether an ID was given and not given back.
func (s *StateIDAllocators) IsLive(id StateID) bool { return s.Status(id) == ids.Live }

// LiveCount is how many IDs of the type are live.
func (s *StateIDAllocators) LiveCount(t StateType) int {
	a, ok := s.byType[t]
	if !ok {
		return 0
	}
	return a.LiveCount()
}

// Clone returns a copy that shares nothing.
func (s *StateIDAllocators) Clone() StateIDAllocators {
	c := StateIDAllocators{
		steps: s.steps, byType: make(map[StateType]*ids.Allocator[uint32], len(s.byType)),
		ordered: slices.Clone(s.ordered),
	}
	for t, a := range s.byType {
		copied := a.Clone()
		c.byType[t] = &copied
	}
	return c
}

// Hash works what it holds into a hash: for each type, its allocator and grave. The live IDs are
// not in it: they are the IDs of what the state holds, which the hash of the state covers, and a
// checkpoint is refused when the two differ.
func (s *StateIDAllocators) Hash(h *Hasher) {
	h.U64(s.steps)
	for _, t := range s.ordered {
		a := s.byType[t]
		h.U64(uint64(t))
		h.U64(uint64(a.Last()))
		if a.Wrapped() {
			h.U64(1)
		} else {
			h.U64(0)
		}
		h.U64(a.Clock())
		for id, until := range a.Grave() {
			h.U64(uint64(id))
			h.U64(until)
		}
	}
}

// Encode writes it: the steps, and then each type with its allocator, from the smallest type.
func (s *StateIDAllocators) Encode(w *binary.Writer) {
	w.Uvarint(s.steps)
	w.Uvarint(uint64(len(s.ordered)))
	for _, t := range s.ordered {
		w.Uint8(uint8(t))
		s.byType[t].Encode(w)
	}
}

// DecodeStateIDAllocators reads what Encode wrote. It refuses a type that is there twice or out of
// order, an allocator with other steps or another limit, and what the package ids refuses.
func DecodeStateIDAllocators(r *binary.Reader) (StateIDAllocators, error) {
	s := NewStateIDAllocators(r.Uvarint())
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return StateIDAllocators{}, err
	}
	if n > uint64(r.Left()) { // a type takes at least one byte
		return StateIDAllocators{}, fmt.Errorf("%w: %d types", binary.ErrShort, n)
	}
	for range n {
		t := StateType(r.Uint8())
		if len(s.ordered) > 0 && t <= s.ordered[len(s.ordered)-1] {
			return StateIDAllocators{}, fmt.Errorf(
				"the state IDs of the type %d are not in order", t,
			)
		}
		a, err := ids.Decode[uint32](r)
		if err != nil {
			return StateIDAllocators{}, fmt.Errorf("the state IDs of the type %d: %w", t, err)
		}
		if a.Steps() != s.steps || a.Limit() != stateLocalLimit {
			return StateIDAllocators{}, fmt.Errorf(
				"the state IDs of the type %d are not made as the rest", t,
			)
		}
		s.byType[t] = &a
		s.ordered = append(s.ordered, t)
	}
	return s, r.Err()
}
