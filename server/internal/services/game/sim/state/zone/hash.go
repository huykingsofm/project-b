package zone

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// Hash returns a number that depends on the data of the state and on nothing else.
func (s *State) Hash() uint64 {
	h := state.NewHasher()
	h.U64(uint64(s.ID))
	h.U64(s.Tick)
	for _, l := range s.ZoneLayers.All() {
		h.U64(uint64(l.ID))
		h.U64(uint64(uint32(l.Capacity)))
	}
	h.U64(uint64(s.Chat.Len()))
	return h.Sum()
}
