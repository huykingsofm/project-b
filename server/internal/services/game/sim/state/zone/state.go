// Package zone holds the state of a zone: the map, and the zonelayers it runs. It follows the same
// rules as the state of a zonelayer: data and nothing else, changed only through its own methods,
// with events of its own.
//
// The zone does not keep count of who is in its zonelayers. An action that needs a count reads it from
// what the zonelayers published.
package zone

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone/event"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/internal/shared/orderedmap"
	"github.com/huykingsofm/project-b/server/internal/shared/table"
)

// ZoneLayerSlot is what a zone knows of one of its zonelayers: how many characters it takes.
type ZoneLayerSlot struct {
	ID       state.ZoneLayerID
	Capacity int32
}

// ChatMessage is something a character said in the zone.
type ChatMessage struct {
	FromID state.CharacterID
	Text   string
	Tick   uint64
}

// Index says by which columns a message is found in a table of messages: who said it.
func (m ChatMessage) Index() map[string]any { return map[string]any{"from": m.FromID} }

// chatKeep is how many messages a zone keeps of what was said in it.
const chatKeep = 100

// State is the state of a zone.
type State struct {
	// Data.
	ID         state.ZoneID
	Tick       uint64
	Tiles      *tilemap.TileMap
	ZoneLayers orderedmap.Map[state.ZoneLayerID, ZoneLayerSlot]
	Chat       table.Table[ChatMessage]

	// Events: not data, never cloned.
	events state.Events[event.Event]
}

// NewState makes the state of a zone with no zonelayer, on a map.
func NewState(zoneID state.ZoneID, tiles *tilemap.TileMap) *State {
	return &State{ID: zoneID, Tiles: tiles}
}

// Clone returns a copy of the data, with an empty event. The copy shares the map, which never changes.
func (s *State) Clone() *State {
	return &State{ID: s.ID, Tick: s.Tick, Tiles: s.Tiles, ZoneLayers: s.ZoneLayers.Clone(), Chat: s.Chat.Clone()}
}

// SetTick sets the tick that the state is at.
func (s *State) SetTick(tick uint64) { s.Tick = tick }

// TakeEvents returns the events of the tick and empties them.
func (s *State) TakeEvents() []event.Event { return s.events.Take() }

// AddZoneLayer makes the zone know a zonelayer with room for capacity characters.
func (s *State) AddZoneLayer(zoneLayerID state.ZoneLayerID, capacity int32) {
	s.ZoneLayers.Set(zoneLayerID, ZoneLayerSlot{ID: zoneLayerID, Capacity: capacity})
}

// SendMessage records a message that was sent in the zone and tells the clients.
func (s *State) SendMessage(m ChatMessage) {
	s.Chat.Add(m)
	s.Chat.Trim(chatKeep)
	s.events.Add(event.ChatMessageSent{FromID: m.FromID, Text: m.Text, Tick: m.Tick})
}
