package zonelayer

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
)

// Hash returns a number that depends on the data of the state and on nothing else: a replay runs
// an engine again on the inputs that were recorded and compares it with the one taken the first
// time. It walks the containers in their own order, which is fixed.
func (s *State) Hash() uint64 {
	h := sim.NewHasher()
	h.U64(uint64(s.id))
	h.U64(uint64(s.zoneID))
	h.U64(uint64(s.worldID))
	h.U64(s.tick)
	state.HashIDAllocator(&h, &s.entityIDAllocator)
	for _, e := range s.entities.All() {
		e.Hash(&h)
	}
	for characterID, entityID := range s.bodies.AllV1() {
		h.U64(uint64(characterID))
		h.U64(uint64(entityID))
	}
	for _, c := range s.characters.All() {
		h.U64(uint64(c.characterID))
		h.String(c.name)
		h.U64(uint64(uint32(c.speed)))
		for key, until := range c.cooldowns.All() {
			h.U64(uint64(key))
			h.U64(until)
		}
	}
	return h.Sum()
}
