// Package zone holds the state of a zone: the map, and the zonelayers it runs. It follows the same
// rules as the state of a zonelayer: data and nothing else, changed only through its own methods,
// with events of its own.
//
// The zone also holds entities that belong to it and not to one of its zonelayers, such as the
// exits of its map: they stand in every zonelayer of the zone at once. The ID of an entity is
// unique in the scope that holds it, so an ID of the zone and an ID of one of its zonelayers can be
// the same number.
//
// The zone does not keep count of who is in its zonelayers. An action that needs a count reads it
// from what the zonelayers published.
package zone

import (
	"iter"
	"slices"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/zone/event"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/shared/orderedmap"
	"github.com/huykingsofm/project-b/shared/table"
)

// ZoneLayerSlot is what a zone knows of one of its zonelayers: how many characters it takes.
type ZoneLayerSlot struct {
	ID       sim.StateID
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
	ID         sim.StateID
	WorldID    sim.StateID
	Adjacent   []sim.StateID // the zones that the exits of this one lead to
	Tick       uint64
	Tiles      *tilemap.TileMap
	Entities   orderedmap.Map[state.EntityID, ExitEntity]
	ZoneLayers orderedmap.Map[sim.StateID, ZoneLayerSlot]
	Chat       table.Table[ChatMessage]

	nextEntityID uint32

	// Events: not data, never cloned.
	events sim.Events[sim.Event]
}

// NewState makes the state of a zone with no zonelayer, on a map. The exits of the map stand in the
// zone as entities.
func NewState(
	zoneID, worldID sim.StateID, tiles *tilemap.TileMap, adjacent ...sim.StateID,
) *State {
	s := &State{ID: zoneID, WorldID: worldID, Adjacent: slices.Clone(adjacent), Tiles: tiles}
	for _, exit := range tiles.Exits() {
		s.nextEntityID++
		id := state.NewEntityID(zoneID, s.nextEntityID)
		s.Entities.Set(id, ExitEntity{
			EntityID: id, Area: exit.Area, ToZoneID: exit.ToZoneID, Arrival: exit.Arrival,
		})
	}
	return s
}

// Clone returns a copy of the data, with an empty event. The copy shares the map, which never
// changes.
func (s *State) Clone() *State {
	return &State{
		ID:           s.ID,
		WorldID:      s.WorldID,
		Adjacent:     slices.Clone(s.Adjacent),
		Tick:         s.Tick,
		Tiles:        s.Tiles,
		Entities:     s.Entities.Clone(),
		ZoneLayers:   s.ZoneLayers.Clone(),
		Chat:         s.Chat.Clone(),
		nextEntityID: s.nextEntityID,
	}
}

// ExitEntityByID returns the entity with the ID when it is an exit.
func (s *State) ExitEntityByID(entityID state.EntityID) (ExitEntity, bool) {
	return s.Entities.Get(entityID)
}

// ReadOnlyZoneState is the read methods of the state of a zone: what an action may use of a copy
// that another engine published, which it must never change (R26, R28).
type ReadOnlyZoneState interface {
	ExitEntityByID(entityID state.EntityID) (ExitEntity, bool)
	World() sim.StateID
	AdjacentZones() []sim.StateID
	TileMap() tilemap.ReadOnlyTileMap
	ZoneLayerSlots() iter.Seq2[sim.StateID, ZoneLayerSlot]
}

var _ ReadOnlyZoneState = (*State)(nil)

// TileMap returns the map of the zone.
func (s *State) TileMap() tilemap.ReadOnlyTileMap { return s.Tiles }

// ZoneLayerSlots returns the zonelayers of the zone, in order, with the room each has.
func (s *State) ZoneLayerSlots() iter.Seq2[sim.StateID, ZoneLayerSlot] {
	return s.ZoneLayers.All()
}

// SetTick sets the tick that the state is at.
func (s *State) SetTick(tick uint64) { s.Tick = tick }

// World is the ID of the world that the zone is in.
func (s *State) World() sim.StateID { return s.WorldID }

// AdjacentZones returns the zones that the exits of the zone lead to. The slice is a view: do not
// change it.
func (s *State) AdjacentZones() []sim.StateID { return s.Adjacent }

// StateID is the ID of the zone, which makes a State of it.
func (s *State) StateID() sim.StateID { return s.ID }

// Type is the type of the state of a zone.
func (s *State) Type() sim.StateType { return state.ZoneType }

// String is the name of the type of the state of a zone.
func (s *State) String() string { return "zone" }

// StateTick is the tick that the state is at.
func (s *State) StateTick() uint64 { return s.Tick }

// Snapshot returns a copy of the data for the other engines to read.
func (s *State) Snapshot() sim.State { return s.Clone() }

// TakeEvents returns the events of the tick and empties them.
func (s *State) TakeEvents() []sim.Event { return s.events.Take() }

// AddZoneLayer makes the zone know a zonelayer with room for capacity characters.
func (s *State) AddZoneLayer(zoneLayerID sim.StateID, capacity int32) {
	s.ZoneLayers.Set(zoneLayerID, ZoneLayerSlot{ID: zoneLayerID, Capacity: capacity})
}

// SendMessage records a message that was sent in the zone and tells the clients.
func (s *State) SendMessage(m ChatMessage) {
	s.Chat.Add(m)
	s.Chat.Trim(chatKeep)
	s.events.Add(event.ChatMessageSent{FromID: m.FromID, Text: m.Text, Tick: m.Tick})
}
