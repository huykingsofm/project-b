package zone

import (
	"github.com/huykingsofm/project-b/platform/sim"
)

// Hash returns a number that depends on the data of the state and on nothing else.
func (s *State) Hash() uint64 {
	h := sim.NewHasher()
	h.U64(uint64(s.id))
	h.U64(uint64(s.worldID))
	h.U64(s.tick)
	for _, zoneID := range s.adjacent {
		h.U64(uint64(zoneID))
	}
	for _, l := range s.zoneLayers.All() {
		h.U64(uint64(l.id))
		h.U64(uint64(uint32(l.capacity)))
	}
	for _, e := range s.entities.All() {
		e.Hash(&h)
	}
	h.U64(uint64(s.chat.Len()))
	return h.Sum()
}
