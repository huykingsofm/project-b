package world

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// Hash returns a number that depends on the data of the state and on nothing else.
func (s *State) Hash() uint64 {
	h := state.NewHasher()
	h.U64(uint64(s.ID))
	h.U64(s.Tick)
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
