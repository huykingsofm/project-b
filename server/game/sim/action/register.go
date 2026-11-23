package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
)

// Register registers every action of the game with the codecs, with the type of state it is for:
// an action that is read back is an action of that state and of no other. A number or a type that
// two actions share is an error.
func Register(c *sim.Codecs) error {
	return sim.Errors(
		c.RegisterAction(decodeWorldChatPrivateSendAction),
		c.RegisterAction(decodeWorldCharacterItemAddAction),
		c.RegisterAction(decodeZoneChatSendAction),
		c.RegisterAction(decodeZoneCharacterEnterAction),
		c.RegisterAction(decodeZoneLayerChatSendAction),
		c.RegisterAction(decodeZoneLayerCharacterEnterAction),
		c.RegisterAction(decodeZoneLayerCharacterMoveAction),
		c.RegisterAction(decodeZoneLayerCharacterNormalAttackTriggerAction),
		c.RegisterAction(decodeZoneLayerCharacterDamageAction),
		c.RegisterAction(decodeZoneLayerCharacterShieldGrantAction),
		c.RegisterAction(decodeZoneLayerCharacterRegenAction),
		c.RegisterAction(decodeZoneLayerCharacterExitToAction),
		c.RegisterAction(decodeZoneLayerCharacterItemPickupAction),
		c.RegisterAction(decodeZoneLayerItemEntityAddAction),
		c.RegisterAction(decodeZoneLayerItemEntityRemoveAction),
	)
}
