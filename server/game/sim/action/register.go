package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
)

// Register registers every action of the game with the codecs, with the type of state it is for:
// an action that is read back is an action of that state and of no other. A number or a type that
// two actions share is an error.
func Register(c *sim.Codecs) error {
	return sim.Errors(
		c.RegisterAction(
			&WorldChatPrivateSendAction{}, decodeWorldChatPrivateSendAction,
		),
		c.RegisterAction(
			&WorldCharacterItemAddAction{}, decodeWorldCharacterItemAddAction,
		),
		c.RegisterAction(&ZoneChatSendAction{}, decodeZoneChatSendAction),
		c.RegisterAction(
			&ZoneCharacterEnterAction{}, decodeZoneCharacterEnterAction,
		),
		c.RegisterAction(
			&ZoneLayerChatSendAction{}, decodeZoneLayerChatSendAction,
		),
		c.RegisterAction(
			&ZoneLayerCharacterEnterAction{}, decodeZoneLayerCharacterEnterAction,
		),
		c.RegisterAction(
			&ZoneLayerCharacterMoveAction{}, decodeZoneLayerCharacterMoveAction,
		),
		c.RegisterAction(
			&ZoneLayerCharacterNormalAttackTriggerAction{},
			decodeZoneLayerCharacterNormalAttackTriggerAction,
		),
		c.RegisterAction(
			&ZoneLayerCharacterDamageAction{}, decodeZoneLayerCharacterDamageAction,
		),
		c.RegisterAction(
			&ZoneLayerCharacterShieldGrantAction{}, decodeZoneLayerCharacterShieldGrantAction,
		),
		c.RegisterAction(
			&ZoneLayerCharacterRegenAction{}, decodeZoneLayerCharacterRegenAction,
		),
		c.RegisterAction(
			&ZoneLayerCharacterExitToAction{}, decodeZoneLayerCharacterExitToAction,
		),
		c.RegisterAction(
			&ZoneLayerCharacterItemPickupAction{}, decodeZoneLayerCharacterItemPickupAction,
		),
		c.RegisterAction(
			&ZoneLayerItemEntityAddAction{}, decodeZoneLayerItemEntityAddAction,
		),
		c.RegisterAction(
			&ZoneLayerItemEntityRemoveAction{}, decodeZoneLayerItemEntityRemoveAction,
		),
	)
}
