package zone

import (
	"errors"
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/shared/binarycodec"
	"github.com/huykingsofm/project-b/shared/table"
)

// Encode writes the data of the state: its ID and tick, its entities, the zonelayers it knows, what
// was said, and the ID the next entity gets. The map is not written: it never changes and the ID of
// the zone finds it. The events are not data (R18) and are not written.
func (s *State) Encode(w *binarycodec.Writer) error {
	w.Uvarint(uint64(s.ID))
	w.Uvarint(uint64(s.WorldID))
	w.Uvarint(s.Tick)
	w.Uvarint(uint64(len(s.Adjacent)))
	for _, zoneID := range s.Adjacent {
		w.Uvarint(uint64(zoneID))
	}
	w.Uvarint(uint64(s.Entities.Len()))
	for id, e := range s.Entities.All() {
		w.Uvarint(uint64(id))
		e.Encode(w)
	}
	w.Uvarint(uint64(s.ZoneLayers.Len()))
	for id, slot := range s.ZoneLayers.All() {
		w.Uvarint(uint64(id))
		w.Varint(int64(slot.Capacity))
	}
	rows := s.Chat.Rows()
	w.Uvarint(uint64(len(rows)))
	for _, m := range rows {
		w.Uvarint(uint64(m.FromID))
		w.String(m.Text)
		w.Uvarint(m.Tick)
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

// DecodeState reads what State.Encode wrote, of the data version given. The map of the zone is the
// one that tilesOf gives for its ID.
func DecodeState(
	r *binarycodec.Reader, version binarycodec.Version,
	tilesOf func(zoneID sim.StateID) *tilemap.TileMap,
) (*State, error) {
	s := &State{ID: sim.StateID(r.Uvarint()), WorldID: sim.StateID(r.Uvarint()), Tick: r.Uvarint()}
	if err := r.Err(); err != nil {
		return nil, err
	}
	if s.Tiles = tilesOf(s.ID); s.Tiles == nil {
		return nil, errors.New("zone: there is no map for the zone")
	}
	n, err := count(r)
	if err != nil {
		return nil, err
	}
	for range n {
		s.Adjacent = append(s.Adjacent, sim.StateID(r.Uvarint()))
	}
	if n, err = count(r); err != nil {
		return nil, err
	}
	for range n {
		id := state.EntityID(r.Uvarint())
		e, err := decodeExitEntity(r, version)
		if err != nil {
			return nil, err
		}
		s.Entities.Set(id, e)
	}
	if n, err = count(r); err != nil {
		return nil, err
	}
	for range n {
		id := sim.StateID(r.Uvarint())
		s.ZoneLayers.Set(id, ZoneLayerSlot{ID: id, Capacity: int32(r.Varint())})
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
	s.nextEntityID = uint32(r.Uvarint())
	return s, r.Err()
}
