package zonelayer

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/shared/binarycodec"
	"github.com/huykingsofm/project-b/shared/table"
)

// Encode writes the data of the state: its IDs and tick, its entities, its characters, what was
// said, which character has which body, and the ID the next entity gets. The events are not data
// (R18) and are not written. It returns an error, and the writer holds a partial state, when an
// entity cannot be written.
func (s *State) Encode(w *binarycodec.Writer) error {
	w.Uvarint(uint64(s.ID))
	w.Uvarint(uint64(s.ZoneID))
	w.Uvarint(uint64(s.WorldID))
	w.Uvarint(s.Tick)
	w.Uvarint(uint64(s.Entities.Len()))
	for id, e := range s.Entities.All() {
		w.Uvarint(uint64(id))
		if err := EncodeEntity(w, e); err != nil {
			return err
		}
	}
	w.Uvarint(uint64(s.Characters.Len()))
	for id, c := range s.Characters.All() {
		w.Uvarint(uint64(id))
		c.Encode(w)
	}
	rows := s.Chat.Rows()
	w.Uvarint(uint64(len(rows)))
	for _, m := range rows {
		w.Uvarint(uint64(m.FromID))
		w.String(m.Text)
		w.Uvarint(m.Tick)
	}
	w.Uvarint(uint64(s.bodies.Len()))
	for characterID, entityID := range s.bodies.AllV1() {
		w.Uvarint(uint64(characterID))
		w.Uvarint(uint64(entityID))
	}
	w.Uvarint(uint64(s.nextEntityID))
	return nil
}

// count reads how many items follow, and refuses a number that claims more than the bytes left can
// hold: an item takes at least one byte.
func count(r *binarycodec.Reader) (uint64, error) {
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return 0, err
	}
	if n > uint64(r.Left()) {
		return 0, fmt.Errorf("%w: %d items", binarycodec.ErrShort, n)
	}
	return n, nil
}

// DecodeState reads what State.Encode wrote, of the data version given.
func DecodeState(r *binarycodec.Reader, version binarycodec.Version) (*State, error) {
	s := &State{
		ID: sim.StateID(r.Uvarint()), ZoneID: sim.StateID(r.Uvarint()),
		WorldID: sim.StateID(r.Uvarint()), Tick: r.Uvarint(),
	}
	n, err := count(r)
	if err != nil {
		return nil, err
	}
	for range n {
		id := state.EntityID(r.Uvarint())
		e, err := DecodeEntity(r, version)
		if err != nil {
			return nil, err
		}
		s.Entities.Set(id, e)
	}
	if n, err = count(r); err != nil {
		return nil, err
	}
	for range n {
		id := state.CharacterID(r.Uvarint())
		c, err := DecodeCharacter(r, version)
		if err != nil {
			return nil, err
		}
		s.Characters.Set(id, c)
	}
	if n, err = count(r); err != nil {
		return nil, err
	}
	rows := make([]ChatMessage, 0, n)
	for range n {
		rows = append(rows, ChatMessage{
			FromID: state.CharacterID(r.Uvarint()), Text: r.String(), Tick: r.Uvarint(),
		})
	}
	s.Chat = *table.Build(rows)
	if n, err = count(r); err != nil {
		return nil, err
	}
	for range n {
		characterID, entityID := state.CharacterID(r.Uvarint()), state.EntityID(r.Uvarint())
		s.bodies.Link(characterID, entityID)
	}
	s.nextEntityID = uint32(r.Uvarint())
	return s, r.Err()
}
