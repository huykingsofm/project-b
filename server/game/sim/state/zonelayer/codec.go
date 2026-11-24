package zonelayer

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/zonelayer/entity"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// Encode writes the data of the state: its IDs and tick, its entities, its characters, what was
// said, which character has which body, and the ID the next entity gets. The events are not data
// (R18) and are not written. It returns an error, and the writer holds a partial state, when an
// entity cannot be written.
func (s *State) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(s.id))
	w.Uvarint(uint64(s.zoneID))
	w.Uvarint(uint64(s.worldID))
	w.Uvarint(s.tick)
	w.Uvarint(uint64(s.entities.Len()))
	for id, e := range s.entities.All() {
		if id.Owner() != s.id {
			return fmt.Errorf("the entity %d is not of the state %d", id, s.id)
		}
		if err := entity.Encode(w, e); err != nil {
			return err
		}
	}
	w.Uvarint(uint64(s.characters.Len()))
	for id, c := range s.characters.All() {
		w.Uvarint(uint64(id))
		c.Encode(w)
	}
	rows := s.chat.Rows()
	w.Uvarint(uint64(len(rows)))
	for _, m := range rows {
		w.Uvarint(uint64(m.fromID))
		w.String(m.text)
		w.Uvarint(m.tick)
	}
	w.Uvarint(uint64(s.bodies.Len()))
	for characterID, entityID := range s.bodies.AllV1() {
		w.Uvarint(uint64(characterID))
		w.Uvarint(uint64(entityID.Local()))
	}
	s.entityIDAllocator.Encode(w)
	return nil
}

// count reads how many items follow, and refuses a number that claims more than the bytes left can
// hold: an item takes at least one byte.
func count(r *binary.Reader) (uint64, error) {
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return 0, err
	}
	if n > uint64(r.Left()) {
		return 0, fmt.Errorf("%w: %d items", binary.ErrShort, n)
	}
	return n, nil
}

// DecodeState reads what State.Encode wrote, of the data version given.
func DecodeState(r *binary.Reader, version codec.Version) (*State, error) {
	id, zoneID := r.Uint[sim.StateID](), r.Uint[sim.StateID]()
	worldID := r.Uint[sim.StateID]()
	s := NewState(id, zoneID, worldID, 0) // its allocator is read below, with its own steps
	s.tick = r.Uvarint()
	n, err := count(r)
	if err != nil {
		return nil, err
	}
	for range n {
		e, err := entity.Decode(r, version, id)
		if err != nil {
			return nil, err
		}
		s.entities.Set(e.ID(), e)
	}
	if n, err = count(r); err != nil {
		return nil, err
	}
	for range n {
		id := r.Uint[state.CharacterID]()
		c, err := DecodeCharacter(r, version)
		if err != nil {
			return nil, err
		}
		s.characters.Set(id, c)
	}
	if n, err = count(r); err != nil {
		return nil, err
	}
	rows := make([]ChatMessage, 0, n)
	for range n {
		rows = append(rows, ChatMessage{
			fromID: r.Uint[state.CharacterID](), text: r.String(), tick: r.Uvarint(),
		})
	}
	for _, row := range rows {
		s.chat.Add(row)
	}
	if n, err = count(r); err != nil {
		return nil, err
	}
	for range n {
		characterID := r.Uint[state.CharacterID]()
		entityID := state.NewEntityID(id, r.Uint[uint32]())
		s.bodies.Link(characterID, entityID)
	}
	if s.entityIDAllocator, err = state.DecodeEntityIDAllocator(id, r); err != nil {
		return nil, err
	}
	if err = r.Err(); err != nil {
		return nil, err
	}
	for _, entityID := range s.entities.Keys() {
		if err = s.entityIDAllocator.MarkLive(entityID); err != nil {
			return nil, err
		}
	}
	return s, nil
}
