// Package world holds the state of the world: the private messages between players and the items of
// the characters. It follows the same rules as the state of a zonelayer: data and nothing else,
// changed only through its own methods, with events of its own.
package world

import (
	"slices"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world/event"
	"github.com/huykingsofm/project-b/server/internal/shared/orderedmap"
	"github.com/huykingsofm/project-b/server/internal/shared/table"
)

// PrivateChatMessage is something a player sent privately to another player. A private message is
// between players, not between characters: a character is not always played by a player.
type PrivateChatMessage struct {
	FromID state.PlayerID
	ToID   state.PlayerID
	Text   string
	Tick   uint64
}

// Index says by which columns a message is found in a table of messages: who sent it and who it is
// for.
func (m PrivateChatMessage) Index() map[string]any {
	return map[string]any{"from": m.FromID, "to": m.ToID}
}

// InventorySlot is one slot of an inventory: a stack of items of one kind. A slot with a count of 0
// is empty.
type InventorySlot struct {
	ItemID state.ItemID
	Count  uint32
}

// Inventory is the slots of one character, in order. It holds a slice, so it has a Clone (R29).
type Inventory struct {
	Slots []InventorySlot
}

// Clone returns a copy that shares nothing with the inventory.
func (i Inventory) Clone() Inventory { return Inventory{Slots: slices.Clone(i.Slots)} }

// chatKeep is how many private messages the world keeps.
const chatKeep = 1000

// State is the state of the world.
type State struct {
	// Data.
	ID      state.WorldID
	Tick    uint64
	Private table.Table[PrivateChatMessage]

	// Inventories holds the slots of every character: the items of a character belong to the world
	// (R28).
	Inventories orderedmap.Map[state.CharacterID, Inventory]

	// Events: not data, never cloned.
	events state.Events[event.Event]
}

// NewState makes the state of an empty world.
func NewState(worldID state.WorldID) *State { return &State{ID: worldID} }

// ReadOnlyWorldState is the read methods of the state of the world: what an action may use of a
// copy that another engine published, which it must never change (R26, R28).
type ReadOnlyWorldState interface {
	Hash() uint64
}

var _ ReadOnlyWorldState = (*State)(nil)

// Clone returns a copy of the data, with an empty event.
func (s *State) Clone() *State {
	return &State{
		ID:          s.ID,
		Tick:        s.Tick,
		Private:     s.Private.Clone(),
		Inventories: s.Inventories.CloneFunc(Inventory.Clone),
	}
}

// SetTick sets the tick that the state is at.
func (s *State) SetTick(tick uint64) { s.Tick = tick }

// TakeEvents returns the events of the tick and empties them.
func (s *State) TakeEvents() []event.Event { return s.events.Take() }

// SendPrivateMessage records a private message between two players and tells the clients.
func (s *State) SendPrivateMessage(m PrivateChatMessage) {
	s.Private.Add(m)
	s.Private.Trim(chatKeep)
	s.events.Add(
		event.PrivateChatMessageSent{FromID: m.FromID, ToID: m.ToID, Text: m.Text, Tick: m.Tick},
	)
}

// AddItemToCharacter puts count items of a kind in the inventory of a character and returns how
// many did not fit. The inventory has slotLimit slots (made when it is first needed, and made
// longer, never shorter, when the limit grows), and a slot holds at most stackLimit items of one
// kind. The items fill the slots that already hold that kind first, and then the empty slots, in
// order. The events tell the clients how many were added.
func (s *State) AddItemToCharacter(
	characterID state.CharacterID,
	itemID state.ItemID,
	count uint32,
	inventorySlotLimit int,
	stackLimit uint32,
) uint32 {
	inventory, _ := s.Inventories.Get(characterID)
	inventory = inventory.Clone()
	for len(inventory.Slots) < inventorySlotLimit {
		inventory.Slots = append(inventory.Slots, InventorySlot{})
	}
	left := topUpInventorySlotsOfKind(inventory.Slots, itemID, count, stackLimit)
	left = fillEmptyInventorySlots(inventory.Slots, itemID, left, stackLimit)
	s.Inventories.Set(characterID, inventory)
	if added := count - left; added > 0 {
		s.events.Add(
			event.CharacterItemAdded{CharacterID: characterID, ItemID: itemID, Count: added},
		)
	}
	return left
}

// topUpInventorySlotsOfKind puts items in the slots that already hold the kind, in order, up to
// stackLimit in each, and returns how many are left.
func topUpInventorySlotsOfKind(
	slots []InventorySlot, itemID state.ItemID, left uint32, stackLimit uint32,
) uint32 {
	for i := range slots {
		if left == 0 {
			return 0
		}
		if slots[i].Count == 0 || slots[i].ItemID != itemID {
			continue
		}
		put := min(left, stackLimit-min(slots[i].Count, stackLimit))
		slots[i].Count += put
		left -= put
	}
	return left
}

// fillEmptyInventorySlots puts items in the empty slots, in order, up to stackLimit in each, and
// returns how many are left.
func fillEmptyInventorySlots(
	slots []InventorySlot, itemID state.ItemID, left uint32, stackLimit uint32,
) uint32 {
	for i := range slots {
		if left == 0 {
			return 0
		}
		if slots[i].Count != 0 {
			continue
		}
		put := min(left, stackLimit)
		if put == 0 {
			return left
		}
		slots[i] = InventorySlot{ItemID: itemID, Count: put}
		left -= put
	}
	return left
}
