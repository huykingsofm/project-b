package state

import (
	"fmt"
	"math"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/ids"
)

// EntityIDAllocator is what a state gives the local numbers of its entities with: from 1 up to what
// 32 bits hold. An entity that leaves gives its number back, and it is given again only after the
// steps of the grave, which must be longer than anything that can still name the entity (an action
// set for later at its ID, for one).
type EntityIDAllocator = ids.Allocator[uint32]

// NewEntityIDAllocator makes the allocator of the entities of a state, with the given steps of the
// grave.
func NewEntityIDAllocator(graveSteps uint64) EntityIDAllocator {
	return ids.New[uint32](graveSteps, math.MaxUint32)
}

// HashIDAllocator works what an allocator holds into a hash. Its live IDs are not in it: they are
// the IDs of what the state holds, which the hash of the state covers already, and a checkpoint is
// refused when the two differ.
func HashIDAllocator(h *sim.Hasher, allocator *ids.Allocator[uint32]) {
	h.U64(allocator.Steps())
	h.U64(uint64(allocator.Last()))
	if allocator.Wrapped() {
		h.U64(1)
	} else {
		h.U64(0)
	}
	h.U64(allocator.Clock())
	for id, until := range allocator.Grave() {
		h.U64(uint64(id))
		h.U64(until)
	}
}

// CheckEntityIDs reports an error when the entities that a state holds are not exactly the live IDs
// of its allocator: a checkpoint in which they differ cannot be one that was written.
func CheckEntityIDs(
	owner sim.StateID, allocator *EntityIDAllocator, entityIDs []EntityID,
) error {
	if allocator.LiveCount() != len(entityIDs) {
		return fmt.Errorf(
			"the state %d holds %d entities and its IDs say %d are live",
			owner, len(entityIDs), allocator.LiveCount(),
		)
	}
	for _, id := range entityIDs {
		if id.Owner() != owner || !allocator.IsLive(id.Local()) {
			return fmt.Errorf("the entity %d is not a live ID of the state %d", id, owner)
		}
	}
	return nil
}

// CheckZoneLayerIDs reports an error when the zonelayers that a world knows are not exactly the
// live zonelayer IDs that it gave: a checkpoint in which they differ cannot be one that was
// written.
func CheckZoneLayerIDs(stateIDAllocators *sim.StateIDAllocators, zoneLayerIDs []sim.StateID) error {
	if live := stateIDAllocators.LiveCount(ZoneLayerType); live != len(zoneLayerIDs) {
		return fmt.Errorf(
			"the world knows %d zonelayers and its IDs say %d are live", len(zoneLayerIDs), live,
		)
	}
	for _, id := range zoneLayerIDs {
		if id.Type() != ZoneLayerType || !stateIDAllocators.IsLive(id) {
			return fmt.Errorf("the zonelayer %d is not a live ID of the world", id)
		}
	}
	return nil
}
