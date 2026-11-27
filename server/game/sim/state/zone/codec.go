package zone

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/zone/entity"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/ring"
)

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
	s := &State{
		worldID: r.Uint[sim.StateID](), tick: r.Uvarint(),
		chat: ring.New[ChatMessage](chatKeep),
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	var err error
	if s.tiles, err = tilemap.Decode(r, version); err != nil {
		return nil, err
	}
	n, err := count(r)
	if err != nil {
		return nil, err
	}
	for range n {
		e, err := entity.Decode(r, version)
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
	s.lastEntityID = r.Uvarint()
	return s, r.Err()
}
