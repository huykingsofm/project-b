package action

import zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"

// The cooldown keys of the actions: the register of what a cooldown can be for. The numbers count
// up from 1 and 0 is no key. A new key takes the next number, at the end of the list, and a number
// is never changed or reused, because the event log writes it. A key is the key of the cooldown
// that an action sets, and another action may read it.
const (
	// ZoneLayerCharacterMoveActionCooldownKey is the key of the cooldown that
	// ZoneLayerCharacterMoveAction sets: the first tick at which the character is not moving any
	// more.
	ZoneLayerCharacterMoveActionCooldownKey zonelayerstate.CooldownKey = iota + 1
	// ZoneLayerCharacterNormalAttackTriggerActionCooldownKey is the key of the cooldown that
	// ZoneLayerCharacterNormalAttackTriggerAction sets: the first tick at which the character can
	// attack again.
	ZoneLayerCharacterNormalAttackTriggerActionCooldownKey
)
