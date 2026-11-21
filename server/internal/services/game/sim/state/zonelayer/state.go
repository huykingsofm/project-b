// Package zonelayer holds the state of a zonelayer: one copy of a map, with the entities in it.
//
// A state is data and nothing else: it holds no function, channel or connection, and it never
// reaches another engine. Only the goroutine of the engine that owns it changes it, and only
// through its methods, which keep the rules of the state and write the events. It has two regions:
// the data, of which the engine publishes a Clone at the end of every tick, and the events (package
// event below this one), what happened in the tick for the clients, which are never cloned or
// published.
package zonelayer

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer/event"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/internal/shared/bimap"
	"github.com/huykingsofm/project-b/server/internal/shared/orderedmap"
	"github.com/huykingsofm/project-b/server/internal/shared/table"
)

// State is the state of a zonelayer.
type State struct {
	// Data.
	ID         state.ZoneLayerID
	ZoneID     state.ZoneID
	Tick       uint64
	Entities   orderedmap.Map[state.EntityID, Entity]
	Characters orderedmap.Map[state.CharacterID, Character]
	Chat       table.Table[ChatMessage]

	// bodies links each character to the entity that is its body, and back.
	bodies       bimap.BiMap[state.CharacterID, state.EntityID]
	nextEntityID state.EntityID

	// Events: not data, never cloned.
	events state.Events[event.Event]
}

// NewState makes the state of an empty zonelayer.
func NewState(zoneLayerID state.ZoneLayerID, zoneID state.ZoneID) *State {
	return &State{ID: zoneLayerID, ZoneID: zoneID}
}

// Clone returns a copy of the data, with an empty event. The copy shares nothing with the state.
func (s *State) Clone() *State {
	return &State{
		ID: s.ID, ZoneID: s.ZoneID, Tick: s.Tick,
		Entities: s.Entities.Clone(),
		Characters: s.Characters.CloneFunc(func(c Character) Character {
			c.Cooldowns = c.Cooldowns.Clone()
			return c
		}),
		Chat:         s.Chat.Clone(),
		bodies:       s.bodies.Clone(),
		nextEntityID: s.nextEntityID,
	}
}

// SetTick sets the tick that the state is at.
func (s *State) SetTick(tick uint64) { s.Tick = tick }

// TakeEvents returns the events of the tick and empties them.
func (s *State) TakeEvents() []event.Event { return s.events.Take() }

// CharacterCount returns how many characters have a body in the zonelayer.
func (s *State) CharacterCount() int { return s.Characters.Len() }

// CharacterByID returns what the zonelayer knows of a character.
func (s *State) CharacterByID(characterID state.CharacterID) (Character, bool) {
	return s.Characters.Get(characterID)
}

// EntityByID returns the entity with the ID.
func (s *State) EntityByID(entityID state.EntityID) (Entity, bool) { return s.Entities.Get(entityID) }

// EntityOfCharacter returns the body of a character.
func (s *State) EntityOfCharacter(characterID state.CharacterID) (Entity, bool) {
	entityID, ok := s.bodies.GetByV1(characterID)
	if !ok {
		return Entity{}, false
	}
	return s.Entities.Get(entityID)
}

// CooldownUntil returns the first tick at which the character is free of what the key K stands
// for, or 0 when the key has no cooldown. A key is an empty struct type that an action defines, and it
// is given as a type argument: s.CooldownUntil[MoveCooldownKey](charID).
func (s *State) CooldownUntil[K comparable](charID state.CharacterID) uint64 {
	c, ok := s.Characters.Get(charID)
	if !ok {
		return 0
	}
	var key K
	until, _ := c.Cooldowns.Get(key)
	return until
}

// SetCooldown sets the first tick at which the character is free of what the key K stands for.
func (s *State) SetCooldown[K comparable](charID state.CharacterID, until uint64) {
	if c := s.Characters.Ptr(charID); c != nil {
		var key K
		c.Cooldowns.Set(key, until)
	}
}

// characterOfEntity returns the character that has the entity as its body.
func (s *State) characterOfEntity(entityID state.EntityID) (Character, bool) {
	characterID, ok := s.bodies.GetByV2(entityID)
	if !ok {
		return Character{}, false
	}
	return s.Characters.Get(characterID)
}

// AttachCharacter gives a character a body at a position, and returns the entity.
func (s *State) AttachCharacter(c Character, at state.Vec) state.EntityID {
	s.nextEntityID++
	e := Entity{ID: s.nextEntityID, Pos: at}
	s.Entities.Set(e.ID, e)
	s.Characters.Set(c.CharID, c)
	s.bodies.Link(c.CharID, e.ID)
	s.events.Add(event.CharacterEntered{EntityID: e.ID, CharID: c.CharID, Name: c.Name, Pos: at})
	return e.ID
}

// DetachCharacter takes a character and its body out of the zonelayer and returns the character, whole.
func (s *State) DetachCharacter(characterID state.CharacterID) (Character, bool) {
	c, ok := s.Characters.Get(characterID)
	if !ok {
		return Character{}, false
	}
	entityID, _ := s.bodies.GetByV1(characterID)
	s.Entities.Delete(entityID)
	s.Characters.Delete(characterID)
	s.bodies.UnlinkByV1(characterID)
	s.events.Add(event.CharacterLeft{EntityID: entityID})
	return c, true
}

// MoveEntity moves an entity up to steps tiles in a direction on the given map and stops at the first
// tile it cannot stand on. It reports whether the entity moved. The map is the one of the zone, which
// the action reads from the state of the zone.
func (s *State) MoveEntity(tiles *tilemap.TileMap, entityID state.EntityID, dir state.Dir, steps int32) bool {
	e := s.Entities.Ptr(entityID)
	if e == nil {
		return false
	}
	if c, ok := s.characterOfEntity(entityID); !ok || c.Health <= 0 {
		return false
	}
	from, to := e.Pos, e.Pos
	for range steps {
		next := to.Add(dir.Vec())
		if !tiles.Free(next) {
			break
		}
		to = next
	}
	if to == from {
		return false
	}
	e.Pos = to
	s.events.Add(event.EntityMoved{EntityID: entityID, From: from, To: to})
	return true
}

// DamageEntity takes health from the character that has the entity as its body, never below zero, and
// reports whether it died.
func (s *State) DamageEntity(entityID state.EntityID, amount int32) bool {
	e := s.Entities.Ptr(entityID)
	if e == nil || amount <= 0 {
		return false
	}
	characterID, _ := s.bodies.GetByV2(entityID)
	c := s.Characters.Ptr(characterID)
	if c == nil || c.Health <= 0 {
		return false
	}
	amount = min(amount, c.Health)
	c.Health -= amount
	s.events.Add(event.EntityHealthChanged{EntityID: entityID, Change: -amount, Health: c.Health})
	if c.Health > 0 {
		return false
	}
	s.events.Add(event.EntityDied{EntityID: entityID})
	return true
}

// HealAllEntities gives every living character some health, up to its most.
func (s *State) HealAllEntities(amount int32) {
	if amount <= 0 {
		return
	}
	for _, characterID := range s.Characters.Keys() {
		c := s.Characters.Ptr(characterID)
		if c.Health <= 0 {
			continue
		}
		gain := min(amount, c.MaxHealth-c.Health)
		if gain <= 0 {
			continue
		}
		c.Health += gain
		entityID, _ := s.bodies.GetByV1(characterID)
		s.events.Add(event.EntityHealthChanged{EntityID: entityID, Change: gain, Health: c.Health})
	}
}

// SendMessage records a message that was sent in the zonelayer and tells the clients.
func (s *State) SendMessage(m ChatMessage) {
	s.Chat.Add(m)
	s.Chat.Trim(chatKeep)
	s.events.Add(event.ChatMessageSent{FromID: m.FromID, Text: m.Text, Tick: m.Tick})
}

// NoticeToCharacter tells a character something.
func (s *State) NoticeToCharacter(characterID state.CharacterID, key string, args ...any) {
	s.events.Add(event.CharacterNoticed{CharID: characterID, Key: key, Args: args})
}
