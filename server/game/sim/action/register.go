package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
)

// Register registers every action of the game with the codecs, with the type of state it is for:
// an action that is read back is an action of that state and of no other. A number or a type that
// two actions share is an error.
func Register(c *sim.Codecs) error {
	return sim.Errors(
		c.RegisterAction[*worldstate.State](
			&WorldChatPrivateSendAction{}, decodeWorldChatPrivateSendAction,
		),
		c.RegisterAction[*worldstate.State](
			&WorldCharacterItemAddAction{}, decodeWorldCharacterItemAddAction,
		),
		c.RegisterAction[*zonestate.State](&ZoneChatSendAction{}, decodeZoneChatSendAction),
		c.RegisterAction[*zonestate.State](
			&ZoneCharacterEnterAction{}, decodeZoneCharacterEnterAction,
		),
		c.RegisterAction[*zonelayerstate.State](
			&ZoneLayerChatSendAction{}, decodeZoneLayerChatSendAction,
		),
		c.RegisterAction[*zonelayerstate.State](
			&ZoneLayerCharacterEnterAction{}, decodeZoneLayerCharacterEnterAction,
		),
		c.RegisterAction[*zonelayerstate.State](
			&ZoneLayerCharacterMoveAction{}, decodeZoneLayerCharacterMoveAction,
		),
		c.RegisterAction[*zonelayerstate.State](
			&ZoneLayerCharacterNormalAttackTriggerAction{},
			decodeZoneLayerCharacterNormalAttackTriggerAction,
		),
		c.RegisterAction[*zonelayerstate.State](
			&ZoneLayerCharacterDamageAction{}, decodeZoneLayerCharacterDamageAction,
		),
		c.RegisterAction[*zonelayerstate.State](
			&ZoneLayerCharacterShieldGrantAction{}, decodeZoneLayerCharacterShieldGrantAction,
		),
		c.RegisterAction[*zonelayerstate.State](
			&ZoneLayerCharacterRegenAction{}, decodeZoneLayerCharacterRegenAction,
		),
		c.RegisterAction[*zonelayerstate.State](
			&ZoneLayerCharacterExitToAction{}, decodeZoneLayerCharacterExitToAction,
		),
		c.RegisterAction[*zonelayerstate.State](
			&ZoneLayerCharacterItemPickupAction{}, decodeZoneLayerCharacterItemPickupAction,
		),
		c.RegisterAction[*zonelayerstate.State](
			&ZoneLayerItemEntityAddAction{}, decodeZoneLayerItemEntityAddAction,
		),
		c.RegisterAction[*zonelayerstate.State](
			&ZoneLayerItemEntityRemoveAction{}, decodeZoneLayerItemEntityRemoveAction,
		),
	)
}
