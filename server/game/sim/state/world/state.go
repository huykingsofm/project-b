// Package world holds the state of the world: the private messages between players and the items of
// the characters. It follows the same rules as the state of a zonelayer: data and nothing else,
// changed only through its own methods, with events of its own.
package world

import (
	"slices"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/world/event"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/logging"
	"github.com/huykingsofm/project-b/shared/ring"
	"github.com/huykingsofm/project-b/shared/stablemap"
)

// chatKeep is how many private messages the world keeps.
const chatKeep = 1000

// The tags that the world gives its requests for engines, which the cluster gives back with the
// answer: what the engine that was made is.
const (
	TagZone      byte = iota + 1 // the engine is a zone of the world
	TagZoneLayer                 // the engine is a zonelayer of a zone of the world
)

// PrivateChatMessage is something a player sent privately to another player. A private message is
// between players, not between characters: a character is not always played by a player.
type PrivateChatMessage struct {
	fromID state.PlayerID
	toID   state.PlayerID
	text   string
	tick   uint64
}

// NewPrivateChatMessage makes a private message that a player said to another at a tick.
func NewPrivateChatMessage(
	fromID, toID state.PlayerID, text string, tick uint64,
) PrivateChatMessage {
	return PrivateChatMessage{
		fromID: fromID, toID: toID,
		text: text, tick: tick,
	}
}

// State is the state of the world.
type State struct {
	// Data.
	tick uint64
	// zones and ZoneLayers are every zone and every zonelayer of the world, in the order they were
	// added: the world is where an engine learns which other engines there are.
	zones       []sim.StateID
	zoneLayers  []sim.StateID
	privateChat ring.Ring[PrivateChatMessage]

	// inventories holds the slots of every character: the items of a character belong to the world
	// (R12).
	inventories stablemap.Map[state.CharacterID, Inventory]

	// Events: not data, never cloned.
	events sim.Events[sim.Event]
}

// NewState makes the state of an empty world.
func NewState() *State {
	return &State{privateChat: ring.New[PrivateChatMessage](chatKeep)}
}

// AdvanceTick takes the state to the next tick.
func (s *State) AdvanceTick() { s.tick++ }

// Type is the type of the state of a world.
func (s *State) Type() sim.StateType { return state.WorldType }

// Tick is the tick that the state is at.
func (s *State) Tick() uint64 { return s.tick }

// Clone returns a copy of the data that shares nothing with the state, and has no event: what the
// engine publishes for the other engines to read.
func (s *State) Clone() sim.State { return s.clone() }

// TakeEvents returns the events of the tick and empties them.
func (s *State) TakeEvents() []sim.Event { return s.events.Take() }

// ZoneIDs returns every zone of the world, in the order they were added. The slice is a view: do
// not change it.
func (s *State) ZoneIDs() []sim.StateID { return s.zones }

// ZoneLayerIDs returns every zonelayer of the world, in the order they were added. The slice is a
// view: do not change it.
func (s *State) ZoneLayerIDs() []sim.StateID { return s.zoneLayers }

// AddZone makes the world know a zone. A zone it knows already is not added again.
func (s *State) AddZone(zoneID sim.StateID) {
	if !slices.Contains(s.zones, zoneID) {
		s.zones = append(s.zones, zoneID)
	}
}

// AddZoneLayer makes the world know a zonelayer, which the cluster gave an ID. It reports false for
// an ID that is not one of a zonelayer, and for one that the world knows already.
func (s *State) AddZoneLayer(zoneLayerID sim.StateID) bool {
	if zoneLayerID.Type() != state.ZoneLayerType || zoneLayerID.Local() == 0 ||
		slices.Contains(s.zoneLayers, zoneLayerID) {
		return false
	}
	s.zoneLayers = append(s.zoneLayers, zoneLayerID)
	return true
}

// RemoveZoneLayer makes the world forget a zonelayer. The ID is not given again. It reports false
// for an ID that is not one of a zonelayer of the world.
func (s *State) RemoveZoneLayer(zoneLayerID sim.StateID) bool {
	if zoneLayerID.Type() != state.ZoneLayerType || !slices.Contains(s.zoneLayers, zoneLayerID) {
		return false
	}
	s.zoneLayers = slices.DeleteFunc(
		s.zoneLayers, func(id sim.StateID) bool { return id == zoneLayerID },
	)
	return true
}

// SendPrivateMessage records a private message between two players and tells the clients.
func (s *State) SendPrivateMessage(m PrivateChatMessage) {
	s.privateChat.Add(m)
	s.events.Add(
		event.PrivateChatMessageSent{FromID: m.fromID, ToID: m.toID, Text: m.text, Tick: m.tick},
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
	inventory, _ := s.inventories.Get(characterID)
	inventory = inventory.Clone()
	for len(inventory.slots) < inventorySlotLimit {
		inventory.slots = append(inventory.slots, InventorySlot{})
	}
	left := topUpInventorySlotsOfKind(inventory.slots, itemID, count, stackLimit)
	left = fillEmptyInventorySlots(inventory.slots, itemID, left, stackLimit)
	s.inventories.Set(characterID, inventory)
	if added := count - left; added > 0 {
		s.events.Add(
			event.CharacterItemAdded{CharacterID: characterID, ItemID: itemID, Count: added},
		)
	}
	return left
}

// PrivateMessageCount is how many private messages the world keeps.
func (s *State) PrivateMessageCount() int { return s.privateChat.Len() }

// ItemsHeld is how many items of a kind a character holds, in all the slots of its inventory.
func (s *State) ItemsHeld(characterID state.CharacterID, itemID state.ItemID) uint32 {
	inventory, _ := s.inventories.Get(characterID)
	var total uint32
	for _, slot := range inventory.slots {
		if slot.count > 0 && slot.itemID == itemID {
			total += slot.count
		}
	}
	return total
}

// InventorySlots returns the slots of the inventory of a character, in order, empty ones included.
func (s *State) InventorySlots(characterID state.CharacterID) []InventorySlot {
	inventory, _ := s.inventories.Get(characterID)
	return slices.Clone(inventory.slots)
}

// Encode writes the data of the state: its tick, the zones and zonelayers it knows, the IDs
// it gives the states it makes, the private messages, and the inventory of every character. The
// events are not data (R9) and are not written.
func (s *State) Encode(w *binary.Writer) error {
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

// Hash returns a number that depends on the data of the state and on nothing else.
func (s *State) Hash() uint64 {
	h := sim.NewHasher()
	h.U64(s.tick)
	for _, id := range s.zones {
		h.U64(uint64(id))
	}
	for _, id := range s.zoneLayers {
		h.U64(uint64(id))
	}
	h.U64(uint64(s.privateChat.Len()))
	for characterID, inventory := range s.inventories.All() {
		h.U64(uint64(characterID))
		for _, inventorySlot := range inventory.slots {
			h.U64(uint64(inventorySlot.itemID))
			h.U64(uint64(inventorySlot.count))
		}
	}
	return h.Sum()
}

// HandleDirective takes the answer of the cluster to a request of the world for an engine: the
// zone or the zonelayer that was made is listed by the world, which is the one that knows which
// zones and zonelayers there are (R12). An engine that could not be made is not listed.
func (s *State) HandleDirective(
	_ *sim.Context, _ *sim.ActionRegistry[*State], d sim.DirectiveResponse,
) {
	if d.Kind != sim.DirectiveCreateEngine || len(d.Tag) == 0 {
		return
	}
	if d.Failure != "" {
		logging.Warn("the world asked for an engine that was not made", "failure", d.Failure)
		return
	}
	switch d.Tag[0] {
	case TagZone:
		s.AddZone(d.Engine)
	case TagZoneLayer:
		s.AddZoneLayer(d.Engine)
	}
}

// clone returns a copy of the data, with an empty event.
func (s *State) clone() *State {
	return &State{
		tick:        s.tick,
		zones:       slices.Clone(s.zones),
		zoneLayers:  slices.Clone(s.zoneLayers),
		privateChat: s.privateChat.Clone(),
		inventories: s.inventories.CloneFunc(Inventory.Clone),
	}
}

var _ ReadOnlyWorldState = (*State)(nil)

var _ sim.DirectiveHandler[*State] = (*State)(nil)

// Inventory is the slots of one character, in order. It holds a slice, so it has a Clone (R13).
type Inventory struct {
	slots []InventorySlot
}

// Clone returns a copy that shares nothing with the inventory.
func (i Inventory) Clone() Inventory {
	return Inventory{slots: slices.Clone(i.slots)}
}

// InventorySlot is one slot of an inventory: a stack of items of one kind. A slot with a count of 0
// is empty.
type InventorySlot struct {
	itemID state.ItemID
	count  uint32
}

// ItemID is the kind of item that the slot holds.
func (i InventorySlot) ItemID() state.ItemID { return i.itemID }

// Count is how many items the slot holds; 0 is an empty slot.
func (i InventorySlot) Count() uint32 { return i.count }

// ReadOnlyWorldState is the read methods of the state of the world: what an action may use of a
// copy that another engine published, which it must never change (R5, R12).
type ReadOnlyWorldState interface {
	Hash() uint64
	ZoneIDs() []sim.StateID
	ZoneLayerIDs() []sim.StateID
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
		if slots[i].count == 0 || slots[i].itemID != itemID {
			continue
		}
		put := min(left, stackLimit-min(slots[i].count, stackLimit))
		slots[i].count += put
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
		if slots[i].count != 0 {
			continue
		}
		put := min(left, stackLimit)
		if put == 0 {
			return left
		}
		slots[i] = InventorySlot{itemID: itemID, count: put}
		left -= put
	}
	return left
}
