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

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/zone/entity"
	"github.com/huykingsofm/project-b/server/game/sim/state/zone/event"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/shared/ring"
	"github.com/huykingsofm/project-b/shared/stablemap"
	"github.com/huykingsofm/project-b/shared/table"
)

// ZoneLayerSlot is what a zone knows of one of its zonelayers: how many characters it takes.
type ZoneLayerSlot struct {
	id       sim.StateID
	capacity int32
}

// ChatMessage is something a character said in the zone.
type ChatMessage struct {
	fromID state.CharacterID
	text   string
	tick   uint64
}

// chatKeep is how many messages a zone keeps of what was said in it.
const chatKeep = 100

// State is the state of a zone.
type State struct {
	// Data.
	id         sim.StateID
	worldID    sim.StateID
	tick       uint64
	tiles      *tilemap.TileMap
	entities   table.Table[state.EntityID, state.Entity]
	zoneLayers stablemap.Map[sim.StateID, ZoneLayerSlot]
	chat       ring.Ring[ChatMessage]

	entityIDCounter state.EntityIDCounter

	// Events: not data, never cloned.
	events sim.Events[sim.Event]
}

// NewState makes the state of a zone with no zonelayer, on a map. The exits of the map stand in the
// zone as entities.
func NewState(zoneID, worldID sim.StateID, tiles *tilemap.TileMap) *State {
	s := &State{
		id: zoneID, worldID: worldID, tiles: tiles,
		chat:            ring.New[ChatMessage](chatKeep),
		entityIDCounter: state.NewEntityIDCounter(zoneID),
	}
	for _, exit := range tiles.Exits() {
		id, ok := s.entityIDCounter.Give()
		if !ok {
			break // a map has no more than a few exits: there is always an ID
		}
		s.entities.Set(id, entity.NewExitEntity(id, exit.Area, exit.ToZoneID, exit.Arrival))
	}
	return s
}

// clone returns a copy of the data, with an empty event. The copy shares the map, which never
// changes.
func (s *State) clone() *State {
	return &State{
		id:              s.id,
		worldID:         s.worldID,
		tick:            s.tick,
		tiles:           s.tiles,
		entities:        s.entities.Clone(),
		zoneLayers:      s.zoneLayers.Clone(),
		chat:            s.chat.Clone(),
		entityIDCounter: s.entityIDCounter.Clone(),
	}
}

// ExitEntityByID returns the entity with the ID when it is an exit.
func (s *State) ExitEntityByID(entityID state.EntityID) (entity.ExitEntity, bool) {
	e, ok := s.entities.Get(entityID)
	if !ok {
		return entity.ExitEntity{}, false
	}
	exit, ok := e.(entity.ExitEntity)
	return exit, ok
}

// ReadOnlyZoneState is the read methods of the state of a zone: what an action may use of a copy
// that another engine published, which it must never change (R26, R28).
type ReadOnlyZoneState interface {
	ExitEntityByID(entityID state.EntityID) (entity.ExitEntity, bool)
	WorldID() sim.StateID
	TileMap() tilemap.ReadOnlyTileMap
	ZoneLayerSlots() iter.Seq2[sim.StateID, ZoneLayerSlot]
}

var _ ReadOnlyZoneState = (*State)(nil)

// TileMap returns the map of the zone.
func (s *State) TileMap() tilemap.ReadOnlyTileMap { return s.tiles }

// ZoneLayerSlots returns the zonelayers of the zone, in order, with the room each has.
func (s *State) ZoneLayerSlots() iter.Seq2[sim.StateID, ZoneLayerSlot] {
	return s.zoneLayers.All()
}

// NextTick takes the state to the next tick.
func (s *State) NextTick() { s.tick++ }

// WorldID is the ID of the world that the zone is in.
func (s *State) WorldID() sim.StateID { return s.worldID }

// ID is the ID of the state.
func (s *State) ID() sim.StateID { return s.id }

// Type is the type of the state of a zone.
func (s *State) Type() sim.StateType { return state.ZoneType }

// Tick is the tick that the state is at.
func (s *State) Tick() uint64 { return s.tick }

// Clone returns a copy of the data that shares nothing with the state, and has no event: what the
// engine publishes for the other engines to read.
func (s *State) Clone() sim.State { return s.clone() }

// TakeEvents returns the events of the tick and empties them.
func (s *State) TakeEvents() []sim.Event { return s.events.Take() }

// AddZoneLayer makes the zone know a zonelayer with room for capacity characters.
func (s *State) AddZoneLayer(zoneLayerID sim.StateID, capacity int32) {
	s.zoneLayers.Set(zoneLayerID, ZoneLayerSlot{id: zoneLayerID, capacity: capacity})
}

// SendMessage records a message that was sent in the zone and tells the clients.
func (s *State) SendMessage(m ChatMessage) {
	s.chat.Add(m)
	s.events.Add(event.ChatMessageSent{FromID: m.fromID, Text: m.text, Tick: m.tick})
}

// NewChatMessage makes a message that a character said in a zone at a tick.
func NewChatMessage(fromID state.CharacterID, text string, tick uint64) ChatMessage {
	return ChatMessage{
		fromID: fromID, text: text, tick: tick,
	}
}

// ID is the ID of the zonelayer.
func (z ZoneLayerSlot) ID() sim.StateID { return z.id }

// Capacity is how many characters the zonelayer takes.
func (z ZoneLayerSlot) Capacity() int32 { return z.capacity }

// ExitEntityCount is how many exits the zone holds.
func (s *State) ExitEntityCount() int { return s.entities.Len() }

// ChatMessageCount is how many chat messages the zone keeps.
func (s *State) ChatMessageCount() int { return s.chat.Len() }

// ZoneLayerCount is how many zonelayers the zone knows.
func (s *State) ZoneLayerCount() int { return s.zoneLayers.Len() }

// RemoveZoneLayer makes the zone forget a zonelayer.
func (s *State) RemoveZoneLayer(zoneLayerID sim.StateID) { s.zoneLayers.Delete(zoneLayerID) }
