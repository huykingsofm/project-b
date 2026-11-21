package zonelayer

import (
	"reflect"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
)

// Hash returns a number that depends on the data of the state and on nothing else: a replay runs
// an engine again on the inputs that were recorded and compares it with the one taken the first
// time. It walks the containers in their own order, which is fixed.
func (s *State) Hash() uint64 {
	h := state.NewHasher()
	h.U64(uint64(s.ID))
	h.U64(s.Tick)
	for _, e := range s.Entities.All() {
		e.Hash(&h)
	}
	for characterID, entityID := range s.bodies.AllV1() {
		h.U64(uint64(characterID))
		h.U64(uint64(entityID))
	}
	for _, c := range s.Characters.All() {
		h.U64(uint64(c.CharacterID))
		h.String(c.Name)
		h.U64(uint64(uint32(c.Speed)))
		for key, until := range c.Cooldowns.All() {
			h.String(reflect.TypeOf(key).String())
			h.U64(until)
		}
	}
	return h.Sum()
}
