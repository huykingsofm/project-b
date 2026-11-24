package world

import (
	"github.com/huykingsofm/project-b/platform/sim"
)

// Hash returns a number that depends on the data of the state and on nothing else.
func (s *State) Hash() uint64 {
	h := sim.NewHasher()
	h.U64(uint64(s.id))
	h.U64(s.tick)
	s.stateIDCounters.Hash(&h)
	for _, id := range s.zones {
		h.U64(uint64(id))
	}
	for _, id := range s.zoneLayers {
		h.U64(uint64(id))
	}
	h.U64(uint64(s.privateChat.Len()))
	for characterID, inventory := range s.inventories.All() {
		h.U64(uint64(characterID))
		for _, inventorySlot := range inventory.slots {
			h.U64(uint64(inventorySlot.itemID))
			h.U64(uint64(inventorySlot.count))
		}
	}
	return h.Sum()
}
