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
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state/entity"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer/event"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/internal/shared/bimap"
	"github.com/huykingsofm/project-b/server/internal/shared/orderedmap"
	"github.com/huykingsofm/project-b/server/internal/shared/table"
	"slices"
)

// State is the state of a zonelayer.
type State struct {
	// Data.
	ID         state.ZoneLayerID
	ZoneID     state.ZoneID
	Tick       uint64
	Entities   orderedmap.Map[state.EntityID, entity.Entity]
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

// ReadOnlyZoneLayerState is the read methods of the state of a zonelayer: what an action may use of
// a copy that another engine published, which it must never change (R26, R28).
type ReadOnlyZoneLayerState interface {
	CharacterCount() int
}

var _ ReadOnlyZoneLayerState = (*State)(nil)

// Clone returns a copy of the data, with an empty event. The copy shares nothing with the state.
func (s *State) Clone() *State {
	return &State{
		ID: s.ID, ZoneID: s.ZoneID, Tick: s.Tick,
		Entities:     s.Entities.Clone(),
		Characters:   s.Characters.Clone(),
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

// CharacterByID returns what the zonelayer knows of a character. The health it says is the health
// of the body of the character, which is where it lives while the character is in the zonelayer.
func (s *State) CharacterByID(characterID state.CharacterID) (Character, bool) {
	c, ok := s.Characters.Get(characterID)
	if !ok {
		return Character{}, false
	}
	if body, ok := s.CharacterEntityByCharacterID(characterID); ok {
		c.Health, c.MaxHealth, c.Radius = body.Hp, body.MaxHp, body.Rad
	}
	return c, true
}

// EntityByID returns the entity with the ID.
func (s *State) EntityByID(entityID state.EntityID) (entity.Entity, bool) {
	return s.Entities.Get(entityID)
}

// ItemEntityByID returns the entity with the ID when it is an item.
func (s *State) ItemEntityByID(entityID state.EntityID) (entity.ItemEntity, bool) {
	item, ok := s.Entities.Get(entityID)
	if !ok {
		return entity.ItemEntity{}, false
	}
	e, ok := item.(entity.ItemEntity)
	return e, ok
}

// CharacterEntityByCharacterID returns the body of a character.
func (s *State) CharacterEntityByCharacterID(
	characterID state.CharacterID,
) (entity.CharacterEntity, bool) {
	entityID, ok := s.bodies.GetByV1(characterID)
	if !ok {
		return entity.CharacterEntity{}, false
	}
	body, ok := s.Entities.Get(entityID)
	if !ok {
		return entity.CharacterEntity{}, false
	}
	e, ok := body.(entity.CharacterEntity)
	return e, ok
}

// CooldownUntil returns the first tick at which the character is free of what the key stands for,
// or 0 when the key has no cooldown.
func (s *State) CooldownUntil(characterID state.CharacterID, key CooldownKey) uint64 {
	c, ok := s.Characters.Get(characterID)
	if !ok {
		return 0
	}
	until, _ := c.Cooldowns.Get(key)
	return until
}

// SetCooldown sets the first tick at which the character is free of what the key stands for.
func (s *State) SetCooldown(characterID state.CharacterID, key CooldownKey, until uint64) {
	if c := s.Characters.Ptr(characterID); c != nil {
		c.Cooldowns.Set(key, until)
	}
}

// AddCharacterEntity gives a character a body at a position, and returns the entity.
func (s *State) AddCharacterEntity(c Character, at state.Vec) state.EntityID {
	s.nextEntityID++
	e := entity.CharacterEntity{
		EntityID: s.nextEntityID, Pos: at, Rad: c.Radius, Hp: c.Health, MaxHp: c.MaxHealth,
	}
	s.Entities.Set(e.EntityID, e)
	// the state owns its copy: what the caller sent, and keeps, is never changed by the engine
	// (R29)
	c = c.Clone()
	c.Health, c.MaxHealth, c.Radius = 0, 0, 0 // from here on they are the body's
	s.Characters.Set(c.CharacterID, c)
	s.bodies.Link(c.CharacterID, e.EntityID)
	s.events.Add(
		event.CharacterEntered{
			EntityID: e.EntityID, CharacterID: c.CharacterID, Name: c.Name, Pos: at, Radius: e.Rad,
		},
	)
	return e.EntityID
}

// RemoveCharacterEntity takes a character and its body out of the zonelayer and returns the
// character, whole.
func (s *State) RemoveCharacterEntity(characterID state.CharacterID) (Character, bool) {
	c, ok := s.CharacterByID(characterID)
	if !ok {
		return Character{}, false
	}
	entityID, _ := s.bodies.GetByV1(characterID)
	body, _ := s.Entities.Get(entityID)
	s.Entities.Delete(entityID)
	s.Characters.Delete(characterID)
	s.bodies.UnlinkByV1(characterID)
	s.events.Add(
		event.CharacterLeft{EntityID: entityID, At: body.Position(), Radius: body.Radius()},
	)
	return c, true
}

// AddItemEntity puts count items of a kind on the ground at a position, for expiry ticks (0: for
// ever), and returns its entity.
func (s *State) AddItemEntity(
	itemID state.ItemID, count uint32, at state.Vec, expiry uint64,
) state.EntityID {
	s.nextEntityID++
	e := entity.ItemEntity{
		EntityID: s.nextEntityID, Pos: at, ItemID: itemID, Count: count, Expiry: expiry,
	}
	s.Entities.Set(e.EntityID, e)
	s.events.Add(event.ItemAppeared{EntityID: e.EntityID, ItemID: itemID, Count: count, Pos: at})
	return e.EntityID
}

// RemoveItemEntity takes an item off the ground and returns it.
func (s *State) RemoveItemEntity(entityID state.EntityID) (entity.ItemEntity, bool) {
	item, ok := s.ItemEntityByID(entityID)
	if !ok {
		return entity.ItemEntity{}, false
	}
	s.Entities.Delete(entityID)
	s.events.Add(event.ItemDisappeared{EntityID: entityID, At: item.Pos})
	return item, true
}

// MoveEntity moves an entity up to distance units in a direction on the given map, as far as its
// body (its radius) can go without touching a tile that cannot be stood on (tilemap.TileMap.Reach).
// It reports whether the entity moved. The map is the one of the zone, which the action reads from
// the state of the zone.
func (s *State) MoveEntity(
	tiles tilemap.ReadOnlyTileMap, entityID state.EntityID, dir state.Dir, distance int32,
) bool {
	e, ok := s.Entities.Get(entityID)
	if !ok {
		return false
	}
	from := e.Position()
	to := tiles.Reach(from, dir, distance, e.Radius())
	if to == from {
		return false
	}
	s.Entities.Set(entityID, e.MovedTo(to))
	s.events.Add(event.EntityMoved{EntityID: entityID, From: from, To: to, Radius: e.Radius()})
	return true
}

// DamageEntity takes health from an entity that can be damaged, never below zero, and reports
// whether it died.
func (s *State) DamageEntity(entityID state.EntityID, amount int32) bool {
	e, ok := s.Entities.Get(entityID)
	if !ok || amount <= 0 {
		return false
	}
	d, ok := e.(entity.DamageableEntity)
	if !ok || d.Health() <= 0 {
		return false
	}
	after := d.OnDamage(amount)
	s.Entities.Set(entityID, after)
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
	for _, entityID := range s.Entities.Keys() {
		e, _ := s.Entities.Get(entityID)
		d, ok := e.(entity.DamageableEntity)
		if !ok || d.Health() <= 0 || d.Health() >= d.MaxHealth() {
			continue
		}
		after := d.OnHeal(amount)
		s.Entities.Set(entityID, after)
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
	s.Chat.Add(m)
	s.Chat.Trim(chatKeep)
	s.events.Add(event.ChatMessageSent{FromID: m.FromID, Text: m.Text, Tick: m.Tick})
}

// NoticeToCharacter tells a character something.
func (s *State) NoticeToCharacter(characterID state.CharacterID, key string, args ...any) {
	// the event goes to another goroutine: it owns its arguments
	s.events.Add(
		event.CharacterNoticed{CharacterID: characterID, Key: key, Args: slices.Clone(args)},
	)
}
