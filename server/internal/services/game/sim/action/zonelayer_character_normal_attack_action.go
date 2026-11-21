package action

import (
	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
)

// normalAttackDamage is 1 for now. Later it will come from the attack of the attacker and the defence of the target.
const normalAttackDamage = 1

var (
	zoneLayerCharacterNormalAttackActionCooldown = config.NewVar("zonelayer.normal_attack.cooldown_ticks", 6, config.Range(1, 600))
)

// ZoneLayerCharacterNormalAttackActionID is the ID of ZoneLayerCharacterNormalAttackAction.
type ZoneLayerCharacterNormalAttackActionID struct{}

// ZoneLayerCharacterNormalAttackActionCooldownKey is the key of the cooldown that
// ZoneLayerCharacterNormalAttackAction sets: the first tick at which the character can attack again.
// Another action may read it.
type ZoneLayerCharacterNormalAttackActionCooldownKey struct{}

// ZoneLayerCharacterNormalAttackAction is the normal attack: a character hitting a target, another
// character of the zonelayer. It does nothing when the target is not in the zonelayer or has no health, and
// when the character is still on the cooldown of the attack. The hit itself is the smallest unit of an
// attack, a ZoneLayerCharacterDamageAction that it adds to the plan behind itself, so effects see the hit
// before it lands.
type ZoneLayerCharacterNormalAttackAction struct {
	CharID   state.CharacterID
	TargetID state.CharacterID // the character it hits
}

func (a *ZoneLayerCharacterNormalAttackAction) ID() ID {
	return ZoneLayerCharacterNormalAttackActionID{}
}
func (a *ZoneLayerCharacterNormalAttackAction) Priority() int16 { return PriorityIntent }
func (a *ZoneLayerCharacterNormalAttackAction) Actor() any      { return a.CharID }

// ApplyZoneLayer sets the cooldown and adds the hit to the plan, or does nothing.
func (a *ZoneLayerCharacterNormalAttackAction) ApplyZoneLayer(c *Context, s *zonelayerstate.State, _ *ZoneLayerRegistry, p *Plan[ZoneLayerAction]) {
	ch, ok := s.CharacterByID(a.CharID)
	if !ok || ch.Health <= 0 {
		return
	}
	source, ok := s.EntityOfCharacter(a.CharID)
	if !ok {
		return
	}
	victim, ok := s.EntityOfCharacter(a.TargetID)
	if !ok {
		return
	}
	if target, _ := s.CharacterByID(a.TargetID); target.Health <= 0 {
		return
	}
	if s.CooldownUntil[ZoneLayerCharacterNormalAttackActionCooldownKey](a.CharID) > c.Tick {
		return
	}
	s.SetCooldown[ZoneLayerCharacterNormalAttackActionCooldownKey](a.CharID, c.Tick+uint64(zoneLayerCharacterNormalAttackActionCooldown.Get(c.Cfg)))
	p.Append(&ZoneLayerCharacterDamageAction{SourceID: source.ID, VictimID: victim.ID, Amount: normalAttackDamage})
}
