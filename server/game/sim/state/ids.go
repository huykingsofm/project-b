package state

import (
	"fmt"
	"math"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/ids"
)

// EntityIDs is what a state gives the local numbers of its entities with: from 1 up to what 32
// bits hold. An entity that leaves gives its number back, and it is given again only after the
// steps of the grave, which must be longer than anything that can still name the entity (an action
// set for later at its ID, for one).
type EntityIDs = ids.Allocator[uint32]

// NewEntityIDs makes the allocator of the entities of a state, with the given steps of the grave.
func NewEntityIDs(graveSteps uint64) EntityIDs {
	return ids.New[uint32](graveSteps, math.MaxUint32)
}

// ZoneLayerIDs is what the world gives the local numbers of the IDs of its zonelayers with: from 1
// up to what the number of a state ID holds. A zonelayer that is removed gives its number back, and
// it is given again only after the steps of the grave, which must be longer than anything that can
// still name the state (a message that waits to be sent to it, for one).
type ZoneLayerIDs = ids.Allocator[uint32]

// NewZoneLayerIDs makes the allocator of the zonelayers of a world, with the given steps of the
// grave.
func NewZoneLayerIDs(graveSteps uint64) ZoneLayerIDs {
	return ids.New[uint32](graveSteps, 1<<24-1)
}

// HashIDs works what an allocator holds into a hash.
func HashIDs(h *sim.Hasher, a *ids.Allocator[uint32]) {
	h.U64(a.Steps())
	h.U64(uint64(a.Largest()))
	h.U64(a.Clock())
	for id, until := range a.Grave() {
		h.U64(uint64(id))
		h.U64(until)
	}
	for id := range a.FreeIDs() {
		h.U64(uint64(id))
	}
}

// CheckEntityIDs reports an error when the entities that a state holds are not exactly the live
// IDs of its allocator: a checkpoint in which they differ cannot be one that was written.
func CheckEntityIDs(owner sim.StateID, a *EntityIDs, entityIDs []EntityID) error {
	if a.LiveCount() != len(entityIDs) {
		return fmt.Errorf(
			"the state %d holds %d entities and its IDs say %d are live",
			owner, len(entityIDs), a.LiveCount(),
		)
	}
	for _, id := range entityIDs {
		if id.Owner() != owner || a.Status(id.Local()) != ids.Live {
			return fmt.Errorf("the entity %d is not a live ID of the state %d", id, owner)
		}
	}
	return nil
}

// CheckZoneLayerIDs reports an error when the zonelayers that a world knows are not exactly the
// live IDs of its allocator: a checkpoint in which they differ cannot be one that was written.
func CheckZoneLayerIDs(a *ZoneLayerIDs, zoneLayerIDs []sim.StateID) error {
	if a.LiveCount() != len(zoneLayerIDs) {
		return fmt.Errorf(
			"the world knows %d zonelayers and its IDs say %d are live",
			len(zoneLayerIDs), a.LiveCount(),
		)
	}
	for _, id := range zoneLayerIDs {
		if id.Type() != ZoneLayerType || a.Status(id.Local()) != ids.Live {
			return fmt.Errorf("the zonelayer %d is not a live ID of the world", id)
		}
	}
	return nil
}
