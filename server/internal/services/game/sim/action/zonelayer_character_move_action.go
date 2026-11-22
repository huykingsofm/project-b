package action

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// ZoneLayerCharacterMoveAction is a run of movement: a character holds a direction for a number of
// ticks. It moves one step in each of them, by setting itself for the next tick until the ticks are
// used up.
//
// It is an action like any other: it is in the plan at every tick it moves in, so an effect can
// change it or cancel it. A run that finds the character still moving does not give up, it waits
// its turn: it takes its place in the cooldown at once (the cooldown moves to the tick its last
// step will be done) and is set for the tick the character is free, which is how runs that arrive
// while another is moving are applied one after the other and never at the same time.
//
// Each tick it moves the character by its speed (Character.Speed, in units a tick); a character
// with no speed does not move. It needs the map to move, and reads it from the state of the zone.
// When the zone has not published its state yet it does nothing in that tick and tries again in the
// next.
type ZoneLayerCharacterMoveAction struct {
	CharacterID state.CharacterID
	Dir         state.Dir
	Ticks       uint32 // ticks still to move

	reserved bool
}

func (a *ZoneLayerCharacterMoveAction) ID() ID          { return ZoneLayerCharacterMoveActionID }
func (a *ZoneLayerCharacterMoveAction) Priority() int16 { return PriorityIntent }
func (a *ZoneLayerCharacterMoveAction) Actor() any      { return a.CharacterID }

// ApplyZoneLayer moves the character one step, or takes its place in the cooldown.
func (a *ZoneLayerCharacterMoveAction) ApplyZoneLayer(
	c *Context, s *zonelayerstate.State, r *ZoneLayerRegistry, _ *Plan[ZoneLayerAction],
) {
	ch, ok := s.CharacterByID(a.CharacterID)
	if !ok {
		return
	}
	entity, _ := s.CharacterEntityByCharacterID(a.CharacterID)
	if ch.Health <= 0 || ch.Speed <= 0 || a.Ticks == 0 {
		return
	}
	if !a.reserved {
		start := max(
			c.Tick, s.CooldownUntil(a.CharacterID, ZoneLayerCharacterMoveActionCooldownKey),
		)
		s.SetCooldown(a.CharacterID, ZoneLayerCharacterMoveActionCooldownKey, start+uint64(a.Ticks))
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
	s.MoveEntity(zone.TileMap(), entity.ID(), a.Dir, ch.Speed)
	a.Ticks--
	if a.Ticks > 0 {
		r.ToSelf(a)
	}
}

// ApplyZoneLayerCancel frees the character of the movement that this run had reserved.
func (a *ZoneLayerCharacterMoveAction) ApplyZoneLayerCancel(
	c *Context, s *zonelayerstate.State, _ *ZoneLayerRegistry,
) {
	if a.reserved &&
		s.CooldownUntil(a.CharacterID, ZoneLayerCharacterMoveActionCooldownKey) > c.Tick {
		s.SetCooldown(a.CharacterID, ZoneLayerCharacterMoveActionCooldownKey, c.Tick)
	}
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a *ZoneLayerCharacterMoveAction) Encode(w *binarycodec.Writer) {
	w.Uvarint(uint64(a.CharacterID))
	w.Uint8(uint8(a.Dir))
	w.Uvarint(uint64(a.Ticks))
	w.Bool(a.reserved)
}

// decodeZoneLayerCharacterMoveAction reads what Encode wrote.
func decodeZoneLayerCharacterMoveAction(
	r *binarycodec.Reader, version binarycodec.Version,
) (Action, error) {
	a := &ZoneLayerCharacterMoveAction{
		CharacterID: state.CharacterID(r.Uvarint()),
		Dir:         state.Dir(r.Uint8()),
		Ticks:       uint32(r.Uvarint()),
		reserved:    r.Bool(),
	}
	return a, r.Err()
}
