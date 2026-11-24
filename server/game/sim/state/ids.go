package state

import (
	"fmt"
	"math"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/ids"
)

// EntityIDAllocator gives the IDs of the entities of one state, its owner: an ID is the ID of the
// owner and a local number from 1 up to what 32 bits hold, from an allocator of the package ids. An
// entity that leaves gives its ID back, and it is given again only after the steps of the grave,
// which must be longer than anything that can still name the entity (an action set for later at its
// ID, for one). It takes and gives EntityID, never a bare number, so an ID of another state is
// refused. Make one with NewEntityIDAllocator.
type EntityIDAllocator struct {
	owner     sim.StateID
	allocator ids.Allocator[uint32]
}

// NewEntityIDAllocator makes the allocator of the entities of the state owner, with the given steps
// of the grave.
func NewEntityIDAllocator(owner sim.StateID, graveSteps uint64) EntityIDAllocator {
	return EntityIDAllocator{owner: owner, allocator: ids.New[uint32](graveSteps, math.MaxUint32)}
}

// Owner is the state whose entities it gives the IDs of.
func (a *EntityIDAllocator) Owner() sim.StateID { return a.owner }

// Give returns an entity ID of the owner that is live from now on. It reports false when the owner
// has no ID left, every one being live or in the grave.
func (a *EntityIDAllocator) Give() (EntityID, bool) {
	local, ok := a.allocator.Give()
	if !ok {
		return 0, false
	}
	return NewEntityID(a.owner, local), true
}

// Release gives a live ID back: it waits in the grave for the steps. It reports false for an ID of
// another state, or one that is not live, which changes nothing.
func (a *EntityIDAllocator) Release(id EntityID) bool {
	return id.Owner() == a.owner && a.allocator.Release(id.Local())
}

// MarkLive makes an ID live again after DecodeEntityIDAllocator: the owner calls it for each entity
// it reads. It refuses an ID of another state, and what the allocator refuses.
func (a *EntityIDAllocator) MarkLive(id EntityID) error {
	if id.Owner() != a.owner {
		return fmt.Errorf("the entity %d is not one of the state %d", id, a.owner)
	}
	if err := a.allocator.MarkLive(id.Local()); err != nil {
		return fmt.Errorf("the entity %d: %w", id, err)
	}
	return nil
}

// IsLive reports whether an ID is one of the owner that was given and not given back.
func (a *EntityIDAllocator) IsLive(id EntityID) bool {
	return id.Owner() == a.owner && a.allocator.IsLive(id.Local())
}

// LiveCount is how many IDs are live.
func (a *EntityIDAllocator) LiveCount() int { return a.allocator.LiveCount() }

// Step takes every ID in the grave one step.
func (a *EntityIDAllocator) Step() { a.allocator.Step() }

// Clone returns a copy that shares nothing.
func (a *EntityIDAllocator) Clone() EntityIDAllocator {
	return EntityIDAllocator{owner: a.owner, allocator: a.allocator.Clone()}
}

// Hash works what it holds into a hash. Its live IDs are not in it: they are the IDs of the
// entities of the owner, which the hash of the state covers already.
func (a *EntityIDAllocator) Hash(h *sim.Hasher) {
	h.U64(a.allocator.Steps())
	h.U64(uint64(a.allocator.Last()))
	if a.allocator.Wrapped() {
		h.U64(1)
	} else {
		h.U64(0)
	}
	h.U64(a.allocator.Clock())
	for id, until := range a.allocator.Grave() {
		h.U64(uint64(id))
		h.U64(until)
	}
}

// Encode writes it, without the owner, which the state that holds it knows, and without the live
// IDs, which are the IDs of its entities.
func (a *EntityIDAllocator) Encode(w *binary.Writer) { a.allocator.Encode(w) }

// DecodeEntityIDAllocator reads what Encode wrote, for the state owner, with no live ID: the owner
// marks each of its entities live with MarkLive. It refuses an allocator whose limit is not the one
// of entity IDs, and what the package ids refuses.
func DecodeEntityIDAllocator(owner sim.StateID, r *binary.Reader) (EntityIDAllocator, error) {
	allocator, err := ids.Decode[uint32](r)
	if err != nil {
		return EntityIDAllocator{}, err
	}
	if allocator.Limit() != math.MaxUint32 {
		return EntityIDAllocator{}, fmt.Errorf(
			"the entity IDs of the state %d have the limit %d", owner, allocator.Limit(),
		)
	}
	return EntityIDAllocator{owner: owner, allocator: allocator}, nil
}
