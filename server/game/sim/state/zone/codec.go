package zone

import (
	"errors"
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/zone/entity"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/ids"
	"github.com/huykingsofm/project-b/shared/ring"
)

// Encode writes the data of the state: its ID and tick, its entities, the zonelayers it knows, what
// was said, and the ID the next entity gets. The map is not written: it never changes and the ID of
// the zone finds it. The events are not data (R18) and are not written.
func (s *State) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(s.id))
	w.Uvarint(uint64(s.worldID))
	w.Uvarint(s.tick)
	w.Uvarint(uint64(len(s.adjacent)))
	for _, zoneID := range s.adjacent {
		w.Uvarint(uint64(zoneID))
	}
	w.Uvarint(uint64(s.entities.Len()))
	for id, e := range s.entities.All() {
		if id.Owner() != s.id {
			return fmt.Errorf("the entity %d is not of the state %d", id, s.id)
		}
		if err := entity.Encode(w, e); err != nil {
			return err
		}
	}
	w.Uvarint(uint64(s.zoneLayers.Len()))
	for id, slot := range s.zoneLayers.All() {
		w.Uvarint(uint64(id))
		w.Varint(int64(slot.capacity))
	}
	rows := s.chat.Rows()
	w.Uvarint(uint64(len(rows)))
	for _, m := range rows {
		w.Uvarint(uint64(m.fromID))
		w.String(m.text)
		w.Uvarint(m.tick)
	}
	s.entityIDs.Encode(w)
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

// DecodeState reads what State.Encode wrote, of the data version given. The map of the zone is the
// one that tilesOf gives for its ID.
func DecodeState(
	r *binary.Reader, version codec.Version,
	tilesOf func(zoneID sim.StateID) *tilemap.TileMap,
) (*State, error) {
	s := &State{
		id: r.Uint[sim.StateID](), worldID: r.Uint[sim.StateID](), tick: r.Uvarint(),
		chat: ring.New[ChatMessage](chatKeep),
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	if s.tiles = tilesOf(s.id); s.tiles == nil {
		return nil, errors.New("zone: there is no map for the zone")
	}
	n, err := count(r)
	if err != nil {
		return nil, err
	}
	for range n {
		s.adjacent = append(s.adjacent, r.Uint[sim.StateID]())
	}
	if n, err = count(r); err != nil {
		return nil, err
	}
	for range n {
		e, err := entity.Decode(r, version, s.id)
		if err != nil {
			return nil, err
		}
		s.entities.Set(e.ID(), e)
	}
	if n, err = count(r); err != nil {
		return nil, err
	}
	for range n {
		id := r.Uint[sim.StateID]()
		s.zoneLayers.Set(id, ZoneLayerSlot{id: id, capacity: r.Int[int32]()})
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
	if s.entityIDs, err = ids.Decode[uint32](r); err != nil {
		return nil, err
	}
	if err = r.Err(); err != nil {
		return nil, err
	}
	if err = state.CheckEntityIDs(s.id, &s.entityIDs, s.entities.Keys()); err != nil {
		return nil, err
	}
	return s, nil
}
