package world

import (
	"github.com/huykingsofm/project-b/platform/sim"
)

// Hash returns a number that depends on the data of the state and on nothing else.
func (s *State) Hash() uint64 {
	h := sim.NewHasher()
	h.U64(uint64(s.ID))
	h.U64(s.Tick)
	for _, id := range s.Zones {
		h.U64(uint64(id))
	}
	for _, id := range s.ZoneLayers {
		h.U64(uint64(id))
	}
	h.U64(uint64(s.Private.Len()))
	for characterID, inventory := range s.Inventories.All() {
		h.U64(uint64(characterID))
		for _, inventorySlot := range inventory.Slots {
			h.U64(uint64(inventorySlot.ItemID))
			h.U64(uint64(inventorySlot.Count))
		}
	}
	return h.Sum()
}
