package state

import (
	"fmt"
	"math"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// EntityIDCounter gives the IDs of the entities of one state, its owner: an ID is the ID of the
// owner and a local number counted up from 1, which is never given again. It takes and gives
// EntityID, never a bare number, so an ID of another state is refused. Make one with
// NewEntityIDCounter.
type EntityIDCounter struct {
	owner sim.StateID
	last  uint64 // the local number last given, or 0
}

// NewEntityIDCounter makes the counter of the entities of the state owner, which has given none.
func NewEntityIDCounter(owner sim.StateID) EntityIDCounter { return EntityIDCounter{owner: owner} }

// Owner is the state whose entities it gives the IDs of.
func (c *EntityIDCounter) Owner() sim.StateID { return c.owner }

// Give returns the next entity ID of the owner. It reports false when the owner has no number
// left: all the 64 bits of a local number are given.
func (c *EntityIDCounter) Give() (EntityID, bool) {
	if c.last == math.MaxUint64 {
		return EntityID{}, false
	}
	c.last++
	return NewEntityID(c.owner, c.last), true
}

// Last is the local number that was last given, or 0 when none was.
func (c *EntityIDCounter) Last() uint64 { return c.last }

// CheckGiven returns an error for an ID that the counter did not give: one of another state, or one
// whose number it has not counted to. A state that is read from a checkpoint checks its entities
// with it.
func (c *EntityIDCounter) CheckGiven(id EntityID) error {
	if id.Owner != c.owner {
		return fmt.Errorf("the entity %v is not one of the state %d", id, c.owner)
	}
	if id.Local == 0 || id.Local > c.last {
		return fmt.Errorf("the entity %v was never given", id)
	}
	return nil
}

// Clone returns a copy that shares nothing.
func (c *EntityIDCounter) Clone() EntityIDCounter { return *c }

// Hash works what it holds into a hash: the number last given.
func (c *EntityIDCounter) Hash(h *sim.Hasher) { h.U64(c.last) }

// Encode writes it, without the owner, which the state that holds it knows.
func (c *EntityIDCounter) Encode(w *binary.Writer) { w.Uvarint(c.last) }

// DecodeEntityIDCounter reads what Encode wrote, for the state owner.
func DecodeEntityIDCounter(owner sim.StateID, r *binary.Reader) (EntityIDCounter, error) {
	last := r.Uvarint()
	if err := r.Err(); err != nil {
		return EntityIDCounter{}, fmt.Errorf("the entity IDs of the state %d: %w", owner, err)
	}
	return EntityIDCounter{owner: owner, last: last}, nil
}
