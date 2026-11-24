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
	if s.entityIDCounter, err = state.DecodeEntityIDCounter(s.id, r); err != nil {
		return nil, err
	}
	if err = r.Err(); err != nil {
		return nil, err
	}
	for _, entityID := range s.entities.Keys() {
		if err = s.entityIDCounter.CheckGiven(entityID); err != nil {
			return nil, err
		}
	}
	return s, nil
}
