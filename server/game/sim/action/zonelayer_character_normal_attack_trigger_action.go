package action

import (
	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// normalAttackDamage is 1 for now. Later it will come from the attack of the attacker and the
// defence of the target.
const normalAttackDamage = 1

var (
	zoneLayerCharacterNormalAttackTriggerActionCooldown = config.NewVar(
		"zonelayer.normal_attack.cooldown_ticks", 6, config.Range(1, 600),
	)
)

// ZoneLayerCharacterNormalAttackTriggerAction is the normal attack: a character hitting a target,
// another character of the zonelayer. It does nothing when the target is not in the zonelayer or
// has no health, and when the character is still on the cooldown of the attack. The hit itself is
// the smallest unit of an attack, a ZoneLayerCharacterDamageAction that it adds to the plan behind
// itself, so effects see the hit before it lands.
type ZoneLayerCharacterNormalAttackTriggerAction struct {
	CharacterID state.CharacterID
	TargetID    state.CharacterID // the character it hits
}

func (a *ZoneLayerCharacterNormalAttackTriggerAction) ID() sim.ActionID {
	return ZoneLayerCharacterNormalAttackTriggerActionID
}
func (a *ZoneLayerCharacterNormalAttackTriggerAction) Priority() int16 { return PriorityIntent }
func (a *ZoneLayerCharacterNormalAttackTriggerAction) Actor() any      { return a.CharacterID }

// Apply sets the cooldown and adds the hit to the plan, or does nothing.
func (a *ZoneLayerCharacterNormalAttackTriggerAction) Apply(
	c *sim.Context, s *zonelayerstate.State, _ *sim.Registry[*zonelayerstate.State],
	p *sim.Plan[*zonelayerstate.State],
) {
	ch, ok := s.CharacterByID(a.CharacterID)
	if !ok || ch.Health() <= 0 {
		return
	}
	source, ok := s.CharacterEntityByCharacterID(a.CharacterID)
	if !ok {
		return
	}
	victim, ok := s.CharacterEntityByCharacterID(a.TargetID)
	if !ok {
		return
	}
	if target, _ := s.CharacterByID(a.TargetID); target.Health() <= 0 {
		return
	}
	if s.CooldownUntil(
		a.CharacterID, ZoneLayerCharacterNormalAttackTriggerActionCooldownKey,
	) > c.Tick {
		return
	}
	s.SetCooldown(
		a.CharacterID,
		ZoneLayerCharacterNormalAttackTriggerActionCooldownKey,
		c.Tick+uint64(zoneLayerCharacterNormalAttackTriggerActionCooldown.Get(c.Cfg)),
	)
	p.Append(
		&ZoneLayerCharacterDamageAction{
			SourceID: source.ID(), VictimID: victim.ID(), Amount: normalAttackDamage,
		},
	)
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a *ZoneLayerCharacterNormalAttackTriggerAction) Encode(w *binary.Writer) {
	w.Uvarint(uint64(a.CharacterID))
	w.Uvarint(uint64(a.TargetID))
}

// decodeZoneLayerCharacterNormalAttackTriggerAction reads what Encode wrote.
func decodeZoneLayerCharacterNormalAttackTriggerAction(
	r *binary.Reader, version codec.Version,
) (sim.Action[*zonelayerstate.State], error) {
	a := &ZoneLayerCharacterNormalAttackTriggerAction{
		CharacterID: r.Uint[state.CharacterID](),
		TargetID:    r.Uint[state.CharacterID](),
	}
	return a, r.Err()
}
