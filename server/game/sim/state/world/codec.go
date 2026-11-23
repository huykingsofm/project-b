package world

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/ring"
)

// Encode writes the data of the state: its ID and tick, the private messages, and the inventory of
// every character. The events are not data (R18) and are not written.
func (s *State) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(s.id))
	w.Uvarint(s.tick)
	w.Uvarint(uint64(len(s.zones)))
	for _, id := range s.zones {
		w.Uvarint(uint64(id))
	}
	w.Uvarint(uint64(len(s.zoneLayers)))
	for _, id := range s.zoneLayers {
		w.Uvarint(uint64(id))
	}
	rows := s.privateChat.Rows()
	w.Uvarint(uint64(len(rows)))
	for _, m := range rows {
		w.Uvarint(uint64(m.fromID))
		w.Uvarint(uint64(m.toID))
		w.String(m.text)
		w.Uvarint(m.tick)
	}
	w.Uvarint(uint64(s.inventories.Len()))
	for characterID, inventory := range s.inventories.All() {
		w.Uvarint(uint64(characterID))
		w.Uvarint(uint64(len(inventory.slots)))
		for _, slot := range inventory.slots {
			w.Uvarint(uint64(slot.itemID))
			w.Uvarint(uint64(slot.count))
		}
	}
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
func DecodeState(r *binary.Reader, _ codec.Version) (*State, error) {
	s := &State{
		id: r.Uint[sim.StateID](), tick: r.Uvarint(),
		privateChat: ring.New[PrivateChatMessage](chatKeep),
	}
	n, err := count(r)
	if err != nil {
		return nil, err
	}
	for range n {
		s.zones = append(s.zones, r.Uint[sim.StateID]())
	}
	if n, err = count(r); err != nil {
		return nil, err
	}
	for range n {
		s.zoneLayers = append(s.zoneLayers, r.Uint[sim.StateID]())
	}
	if n, err = count(r); err != nil {
		return nil, err
	}
	rows := make([]PrivateChatMessage, 0, n)
	for range n {
		rows = append(rows, PrivateChatMessage{
			fromID: r.Uint[state.PlayerID](), toID: r.Uint[state.PlayerID](),
			text: r.String(), tick: r.Uvarint(),
		})
	}
	for _, row := range rows {
		s.privateChat.Add(row)
	}
	if n, err = count(r); err != nil {
		return nil, err
	}
	for range n {
		characterID := r.Uint[state.CharacterID]()
		slots, err := count(r)
		if err != nil {
			return nil, err
		}
		inventory := Inventory{slots: make([]InventorySlot, 0, slots)}
		for range slots {
			inventory.slots = append(
				inventory.slots,
				InventorySlot{itemID: r.Uint[state.ItemID](), count: r.Uint[uint32]()},
			)
		}
		s.inventories.Set(characterID, inventory)
	}
	return s, r.Err()
}
