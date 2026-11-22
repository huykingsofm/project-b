package zone

import (
	"github.com/huykingsofm/project-b/platform/sim"
)

// Hash returns a number that depends on the data of the state and on nothing else.
func (s *State) Hash() uint64 {
	h := sim.NewHasher()
	h.U64(uint64(s.ID))
	h.U64(uint64(s.WorldID))
	h.U64(s.Tick)
	for _, zoneID := range s.Adjacent {
		h.U64(uint64(zoneID))
	}
	for _, l := range s.ZoneLayers.All() {
		h.U64(uint64(l.ID))
		h.U64(uint64(uint32(l.Capacity)))
	}
	for _, e := range s.Entities.All() {
		e.Hash(&h)
	}
	h.U64(uint64(s.Chat.Len()))
	return h.Sum()
}
