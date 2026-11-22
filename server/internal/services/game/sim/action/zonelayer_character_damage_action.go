package action

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
)

// ZoneLayerCharacterDamageAction takes health from an entity. It is in the plan after the action that caused it
// and after the effects that may change it, and an effect that runs before it may lower it or
// cancel it.
type ZoneLayerCharacterDamageAction struct {
	SourceID, VictimID state.EntityID
	Amount             int32
}

func (a *ZoneLayerCharacterDamageAction) ID() ID          { return ZoneLayerCharacterDamageActionID }
func (a *ZoneLayerCharacterDamageAction) Priority() int16 { return PriorityResolve }

// ApplyZoneLayer hurts the victim.
func (a *ZoneLayerCharacterDamageAction) ApplyZoneLayer(_ *Context, s *zonelayerstate.State, _ *ZoneLayerRegistry, _ *Plan[ZoneLayerAction]) {
	if a.Amount <= 0 {
		return
	}
	s.DamageEntity(a.VictimID, a.Amount)
}
