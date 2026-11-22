package world

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/shared/binarycodec"
	"github.com/huykingsofm/project-b/shared/table"
)

// Encode writes the data of the state: its ID and tick, the private messages, and the inventory of
// every character. The events are not data (R18) and are not written.
func (s *State) Encode(w *binarycodec.Writer) error {
	w.Uvarint(uint64(s.ID))
	w.Uvarint(s.Tick)
	w.Uvarint(uint64(len(s.Zones)))
	for _, id := range s.Zones {
		w.Uvarint(uint64(id))
	}
	w.Uvarint(uint64(len(s.ZoneLayers)))
	for _, id := range s.ZoneLayers {
		w.Uvarint(uint64(id))
	}
	rows := s.Private.Rows()
	w.Uvarint(uint64(len(rows)))
	for _, m := range rows {
		w.Uvarint(uint64(m.FromID))
		w.Uvarint(uint64(m.ToID))
		w.String(m.Text)
		w.Uvarint(m.Tick)
	}
	w.Uvarint(uint64(s.Inventories.Len()))
	for characterID, inventory := range s.Inventories.All() {
		w.Uvarint(uint64(characterID))
		w.Uvarint(uint64(len(inventory.Slots)))
		for _, slot := range inventory.Slots {
			w.Uvarint(uint64(slot.ItemID))
			w.Uvarint(uint64(slot.Count))
		}
	}
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
func DecodeState(r *binarycodec.Reader, _ binarycodec.Version) (*State, error) {
	s := &State{ID: sim.StateID(r.Uvarint()), Tick: r.Uvarint()}
	n, err := count(r)
	if err != nil {
		return nil, err
	}
	for range n {
		s.Zones = append(s.Zones, sim.StateID(r.Uvarint()))
	}
	if n, err = count(r); err != nil {
		return nil, err
	}
	for range n {
		s.ZoneLayers = append(s.ZoneLayers, sim.StateID(r.Uvarint()))
	}
	if n, err = count(r); err != nil {
		return nil, err
	}
	rows := make([]PrivateChatMessage, 0, n)
	for range n {
		rows = append(rows, PrivateChatMessage{
			FromID: state.PlayerID(r.Uvarint()), ToID: state.PlayerID(r.Uvarint()),
			Text: r.String(), Tick: r.Uvarint(),
		})
	}
	s.Private = *table.Build(rows)
	if n, err = count(r); err != nil {
		return nil, err
	}
	for range n {
		characterID := state.CharacterID(r.Uvarint())
		slots, err := count(r)
		if err != nil {
			return nil, err
		}
		inventory := Inventory{Slots: make([]InventorySlot, 0, slots)}
		for range slots {
			inventory.Slots = append(
				inventory.Slots,
				InventorySlot{ItemID: state.ItemID(r.Uvarint()), Count: uint32(r.Uvarint())},
			)
		}
		s.Inventories.Set(characterID, inventory)
	}
	return s, r.Err()
}
