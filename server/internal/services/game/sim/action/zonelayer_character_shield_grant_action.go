package action

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
)

// ZoneLayerCharacterShieldGrantAction is a standing action: it stands in the plan at every tick, by setting itself
// for the next tick, until its time is up or it has blocked a damage. In each tick it runs in the modifier
// band, before the damage lands, finds the damage in the plan through the plan's index by ID, and
// cancels the first damage that is aimed at the entity of its character. It is the pattern that
// every effect that changes an action it was not written for follows.
type ZoneLayerCharacterShieldGrantAction struct {
	CharacterID state.CharacterID
	Duration    uint64 // for how many ticks it stands, counted from the first tick it runs

	started bool
	until   uint64
}

func (a *ZoneLayerCharacterShieldGrantAction) ID() ID          { return ZoneLayerCharacterShieldGrantActionID }
func (a *ZoneLayerCharacterShieldGrantAction) Priority() int16 { return PriorityModifier }
func (a *ZoneLayerCharacterShieldGrantAction) Actor() any      { return a.CharacterID }

// ApplyZoneLayer blocks the first damage that is aimed at the character, or stands again next tick.
func (a *ZoneLayerCharacterShieldGrantAction) ApplyZoneLayer(c *Context, s *zonelayerstate.State, r *ZoneLayerRegistry, p *Plan[ZoneLayerAction]) {
	if !a.started {
		a.started, a.until = true, c.Tick+a.Duration
	}
	if c.Tick >= a.until {
		return
	}
	entity, ok := s.CharacterEntityByCharacterID(a.CharacterID)
	if !ok {
		return
	}
	for _, hit := range p.FindType[*ZoneLayerCharacterDamageAction]() {
		if hit.Action.VictimID == entity.ID() {
			p.Cancel(hit.Entry)
			return
		}
	}
	r.ToSelf(a)
}
