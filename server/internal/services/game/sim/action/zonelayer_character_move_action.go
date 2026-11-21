package action

import (
	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
)

var zoneLayerCharacterMoveActionStep = config.NewVar("zonelayer.move.step", 1, config.Range(1, 8))

// ZoneLayerCharacterMoveActionID is the ID of ZoneLayerCharacterMoveAction.
type ZoneLayerCharacterMoveActionID struct{}

// ZoneLayerCharacterMoveActionCooldownKey is the key of the cooldown that ZoneLayerCharacterMoveAction sets: the first
// tick at which the character is not moving any more. Another action may read it.
type ZoneLayerCharacterMoveActionCooldownKey struct{}

// ZoneLayerCharacterMoveAction is a run of movement: a character holds a direction for a number of ticks. It
// moves one step in each of them, by setting itself for the next tick until the ticks are used up.
//
// It is an action like any other: it is in the plan at every tick it moves in, so an effect can
// change it or cancel it. A run that finds the character still moving does not give up, it waits
// its turn: it takes its place in the cooldown at once (the cooldown moves to the tick its last
// step will be done) and is set for the tick the character is free, which is how runs that arrive
// while another is moving are applied one after the other and never at the same time.
//
// It needs the map to move, and reads it from the state of the zone. When the zone has not
// published its state yet it does nothing in that tick and tries again in the next.
type ZoneLayerCharacterMoveAction struct {
	CharID state.CharacterID
	Dir    state.Dir
	Ticks  uint32 // ticks still to move

	reserved bool
}

func (a *ZoneLayerCharacterMoveAction) ID() ID          { return ZoneLayerCharacterMoveActionID{} }
func (a *ZoneLayerCharacterMoveAction) Priority() int16 { return PriorityIntent }
func (a *ZoneLayerCharacterMoveAction) Actor() any      { return a.CharID }

// ApplyZoneLayer moves the character one step, or takes its place in the cooldown.
func (a *ZoneLayerCharacterMoveAction) ApplyZoneLayer(c *Context, s *zonelayerstate.State, r *ZoneLayerRegistry, _ *Plan[ZoneLayerAction]) {
	ch, ok := s.CharacterByID(a.CharID)
	if !ok {
		return
	}
	entity, _ := s.EntityOfCharacter(a.CharID)
	if ch.Health <= 0 || a.Ticks == 0 {
		return
	}
	if !a.reserved {
		start := max(c.Tick, s.CooldownUntil[ZoneLayerCharacterMoveActionCooldownKey](a.CharID))
		s.SetCooldown[ZoneLayerCharacterMoveActionCooldownKey](a.CharID, start+uint64(a.Ticks))
		a.reserved = true
		if start > c.Tick {
			r.ToSelfAt(start, a)
			return
		}
	}
	zone := r.Stable.Zone(s.ZoneID)
	if zone == nil {
		r.ToSelf(a)
		return
	}
	s.MoveEntity(zone.Tiles, entity.ID, a.Dir, int32(zoneLayerCharacterMoveActionStep.Get(c.Cfg)))
	a.Ticks--
	if a.Ticks > 0 {
		r.ToSelf(a)
	}
}

// ApplyZoneLayerCancel frees the character of the movement that this run had reserved.
func (a *ZoneLayerCharacterMoveAction) ApplyZoneLayerCancel(c *Context, s *zonelayerstate.State, _ *ZoneLayerRegistry) {
	if a.reserved && s.CooldownUntil[ZoneLayerCharacterMoveActionCooldownKey](a.CharID) > c.Tick {
		s.SetCooldown[ZoneLayerCharacterMoveActionCooldownKey](a.CharID, c.Tick)
	}
}
