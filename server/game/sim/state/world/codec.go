package world

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/ring"
)

// DecodeState reads what State.Encode wrote, of the data version given.
func DecodeState(r *binary.Reader, _ codec.Version) (*State, error) {
	s := &State{tick: r.Uvarint(), privateChat: ring.New[PrivateChatMessage](chatKeep)}
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
	if err = r.Err(); err != nil {
		return nil, err
	}
	seen := make(map[sim.StateID]bool, len(s.zoneLayers))
	for _, zoneLayerID := range s.zoneLayers {
		if zoneLayerID.Type() != state.ZoneLayerType {
			return nil, fmt.Errorf("the zonelayer %d is a state of another type", zoneLayerID)
		}
		if zoneLayerID.Local() == 0 {
			return nil, fmt.Errorf("the zonelayer %d has the number 0, never given", zoneLayerID)
		}
		if seen[zoneLayerID] {
			return nil, fmt.Errorf("the zonelayer %d is there twice", zoneLayerID)
		}
		seen[zoneLayerID] = true
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
