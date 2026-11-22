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
	"slices"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/zonelayer/event"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/shared/bimap"
	"github.com/huykingsofm/project-b/shared/orderedmap"
	"github.com/huykingsofm/project-b/shared/table"
)

// State is the state of a zonelayer.
type State struct {
	// Data.
	id         sim.StateID
	zoneID     sim.StateID
	worldID    sim.StateID
	tick       uint64
	entities   orderedmap.Map[state.EntityID, Entity]
	characters orderedmap.Map[state.CharacterID, Character]
	chat       table.Table[ChatMessage]

	// bodies links each character to the entity that is its body, and back.
	bodies       bimap.BiMap[state.CharacterID, state.EntityID]
	nextEntityID uint32

	// Events: not data, never cloned.
	events sim.Events[sim.Event]
}

// NewState makes the state of an empty zonelayer.
func NewState(zoneLayerID, zoneID, worldID sim.StateID) *State {
	return &State{id: zoneLayerID, zoneID: zoneID, worldID: worldID}
}

// ReadOnlyZoneLayerState is the read methods of the state of a zonelayer: what an action may use of
// a copy that another engine published, which it must never change (R26, R28).
type ReadOnlyZoneLayerState interface {
	CharacterCount() int
}

var _ ReadOnlyZoneLayerState = (*State)(nil)

// clone returns a copy of the data, with an empty event. The copy shares nothing with the state.
func (s *State) clone() *State {
	return &State{
		id: s.id, zoneID: s.zoneID, worldID: s.worldID, tick: s.tick,
		entities:     s.entities.Clone(),
		characters:   s.characters.Clone(),
		chat:         s.chat.Clone(),
		bodies:       s.bodies.Clone(),
		nextEntityID: s.nextEntityID,
	}
}

// SetTick sets the tick that the state is at.
func (s *State) SetTick(tick uint64) { s.tick = tick }

// ID is the ID of the state.
func (s *State) ID() sim.StateID { return s.id }

// Type is the type of the state of a zonelayer.
func (s *State) Type() sim.StateType { return state.ZoneLayerType }

// String is the name of the type of the state of a zonelayer.
func (s *State) String() string { return "zonelayer" }

// Tick is the tick that the state is at.
func (s *State) Tick() uint64 { return s.tick }

// Clone returns a copy of the data that shares nothing with the state, and has no event: what the
// engine publishes for the other engines to read.
func (s *State) Clone() sim.State { return s.clone() }

// TakeEvents returns the events of the tick and empties them.
func (s *State) TakeEvents() []sim.Event { return s.events.Take() }

// Notice tells a character something, when the actor is one: it is what an engine calls when a
// message that is about a character could not be delivered.
func (s *State) Notice(actor any, key string, args ...any) {
	if characterID, ok := actor.(state.CharacterID); ok {
		s.NoticeToCharacter(characterID, key, args...)
	}
}

// CharacterCount returns how many characters have a body in the zonelayer.
func (s *State) CharacterCount() int { return s.characters.Len() }

// CharacterByID returns what the zonelayer knows of a character. The health it says is the health
// of the body of the character, which is where it lives while the character is in the zonelayer.
func (s *State) CharacterByID(characterID state.CharacterID) (Character, bool) {
	c, ok := s.characters.Get(characterID)
	if !ok {
		return Character{}, false
	}
	if body, ok := s.CharacterEntityByCharacterID(characterID); ok {
		c.health, c.maxHealth, c.radius = body.hp, body.maxHp, body.rad
	}
	return c, true
}

// EntityByID returns the entity with the ID.
func (s *State) EntityByID(entityID state.EntityID) (Entity, bool) {
	return s.entities.Get(entityID)
}

// ItemEntityByID returns the entity with the ID when it is an item.
func (s *State) ItemEntityByID(entityID state.EntityID) (ItemEntity, bool) {
	item, ok := s.entities.Get(entityID)
	if !ok {
		return ItemEntity{}, false
	}
	e, ok := item.(ItemEntity)
	return e, ok
}

// CharacterEntityByCharacterID returns the body of a character.
func (s *State) CharacterEntityByCharacterID(
	characterID state.CharacterID,
) (CharacterEntity, bool) {
	entityID, ok := s.bodies.GetByV1(characterID)
	if !ok {
		return CharacterEntity{}, false
	}
	body, ok := s.entities.Get(entityID)
	if !ok {
		return CharacterEntity{}, false
	}
	e, ok := body.(CharacterEntity)
	return e, ok
}

// CooldownUntil returns the first tick at which the character is free of what the key stands for,
// or 0 when the key has no cooldown.
func (s *State) CooldownUntil(characterID state.CharacterID, key CooldownKey) uint64 {
	c, ok := s.characters.Get(characterID)
	if !ok {
		return 0
	}
	until, _ := c.cooldowns.Get(key)
	return until
}

// SetCooldown sets the first tick at which the character is free of what the key stands for.
func (s *State) SetCooldown(characterID state.CharacterID, key CooldownKey, until uint64) {
	if c := s.characters.Ptr(characterID); c != nil {
		c.cooldowns.Set(key, until)
	}
}

// AddCharacterEntity gives a character a body at a position, and returns the
func (s *State) AddCharacterEntity(c Character, at state.Vec) state.EntityID {
	s.nextEntityID++
	e := CharacterEntity{
		entityID: state.NewEntityID(s.id, s.nextEntityID),
		pos:      at, rad: c.radius, hp: c.health, maxHp: c.maxHealth,
	}
	s.entities.Set(e.entityID, e)
	// the state owns its copy: what the caller sent, and keeps, is never changed by the engine
	// (R29)
	c = c.Clone()
	c.health, c.maxHealth, c.radius = 0, 0, 0 // from here on they are the body's
	s.characters.Set(c.characterID, c)
	s.bodies.Link(c.characterID, e.entityID)
	s.events.Add(
		event.CharacterEntered{
			EntityID: e.entityID, CharacterID: c.characterID, Name: c.name, Pos: at, Radius: e.rad,
		},
	)
	return e.entityID
}

// RemoveCharacterEntity takes a character and its body out of the zonelayer and returns the
// character, whole.
func (s *State) RemoveCharacterEntity(characterID state.CharacterID) (Character, bool) {
	c, ok := s.CharacterByID(characterID)
	if !ok {
		return Character{}, false
	}
	entityID, _ := s.bodies.GetByV1(characterID)
	body, _ := s.entities.Get(entityID)
	s.entities.Delete(entityID)
	s.characters.Delete(characterID)
	s.bodies.UnlinkByV1(characterID)
	s.events.Add(
		event.CharacterLeft{EntityID: entityID, At: body.Position(), Radius: body.Radius()},
	)
	return c, true
}

// AddItemEntity puts count items of a kind on the ground at a position, for expiry ticks (0: for
// ever), and returns its
func (s *State) AddItemEntity(
	itemID state.ItemID, count uint32, at state.Vec, expiry uint64,
) state.EntityID {
	s.nextEntityID++
	e := ItemEntity{
		entityID: state.NewEntityID(s.id, s.nextEntityID),
		pos:      at, itemID: itemID, count: count, expiry: expiry,
	}
	s.entities.Set(e.entityID, e)
	s.events.Add(event.ItemAppeared{EntityID: e.entityID, ItemID: itemID, Count: count, Pos: at})
	return e.entityID
}

// RemoveItemEntity takes an item off the ground and returns it.
func (s *State) RemoveItemEntity(entityID state.EntityID) (ItemEntity, bool) {
	item, ok := s.ItemEntityByID(entityID)
	if !ok {
		return ItemEntity{}, false
	}
	s.entities.Delete(entityID)
	s.events.Add(event.ItemDisappeared{EntityID: entityID, At: item.pos})
	return item, true
}

// MoveEntity moves an entity up to distance units in a direction on the given map, as far as its
// body (its radius) can go without touching a tile that cannot be stood on (tilemap.TileMap.Reach).
// It reports whether the entity moved. The map is the one of the zone, which the action reads from
// the state of the zone.
func (s *State) MoveEntity(
	tiles tilemap.ReadOnlyTileMap, entityID state.EntityID, dir state.Dir, distance int32,
) bool {
	e, ok := s.entities.Get(entityID)
	if !ok {
		return false
	}
	from := e.Position()
	to := tiles.Reach(from, dir, distance, e.Radius())
	if to == from {
		return false
	}
	s.entities.Set(entityID, e.MovedTo(to))
	s.events.Add(event.EntityMoved{EntityID: entityID, From: from, To: to, Radius: e.Radius()})
	return true
}

// DamageEntity takes health from an entity that can be damaged, never below zero, and reports
// whether it died.
func (s *State) DamageEntity(entityID state.EntityID, amount int32) bool {
	e, ok := s.entities.Get(entityID)
	if !ok || amount <= 0 {
		return false
	}
	d, ok := e.(DamageableEntity)
	if !ok || d.Health() <= 0 {
		return false
	}
	after := d.OnDamage(amount)
	s.entities.Set(entityID, after)
	s.events.Add(
		event.EntityHealthChanged{
			EntityID: entityID,
			Change:   after.Health() - d.Health(),
			Health:   after.Health(),
			At:       e.Position(),
			Radius:   e.Radius(),
		},
	)
	if after.Health() > 0 {
		return false
	}
	s.events.Add(event.EntityDied{EntityID: entityID, At: e.Position(), Radius: e.Radius()})
	return true
}

// HealAllEntities gives every living entity that can be damaged some health, up to its most.
func (s *State) HealAllEntities(amount int32) {
	if amount <= 0 {
		return
	}
	for _, entityID := range s.entities.Keys() {
		e, _ := s.entities.Get(entityID)
		d, ok := e.(DamageableEntity)
		if !ok || d.Health() <= 0 || d.Health() >= d.MaxHealth() {
			continue
		}
		after := d.OnHeal(amount)
		s.entities.Set(entityID, after)
		s.events.Add(
			event.EntityHealthChanged{
				EntityID: entityID,
				Change:   after.Health() - d.Health(),
				Health:   after.Health(),
				At:       e.Position(),
				Radius:   e.Radius(),
			},
		)
	}
}

// SendMessage records a message that was sent in the zonelayer and tells the clients.
func (s *State) SendMessage(m ChatMessage) {
	s.chat.Add(m)
	s.chat.Trim(chatKeep)
	s.events.Add(event.ChatMessageSent{FromID: m.fromID, Text: m.text, Tick: m.tick})
}

// NoticeToCharacter tells a character something.
func (s *State) NoticeToCharacter(characterID state.CharacterID, key string, args ...any) {
	// the event goes to another goroutine: it owns its arguments
	s.events.Add(
		event.CharacterNoticed{CharacterID: characterID, Key: key, Args: slices.Clone(args)},
	)
}

// ZoneID is the ID of the zone that the zonelayer is a copy of.
func (s *State) ZoneID() sim.StateID { return s.zoneID }

// WorldID is the ID of the world that the zonelayer is in.
func (s *State) WorldID() sim.StateID { return s.worldID }

// EntityCount is how many entities stand in the zonelayer.
func (s *State) EntityCount() int { return s.entities.Len() }

// EntityIDs returns the IDs of the entities in the zonelayer, in the order they were added.
func (s *State) EntityIDs() []state.EntityID { return s.entities.Keys() }

// ChatMessageCount is how many chat messages the zonelayer keeps.
func (s *State) ChatMessageCount() int { return s.chat.Len() }
