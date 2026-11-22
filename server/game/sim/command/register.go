package command

import (
	"github.com/huykingsofm/project-b/platform/sim"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
)

// Register registers every command of the game with the codecs, with the type of state it is for:
// a command that is read back is a command of that state and of no other. A number or a type that
// two commands share is an error.
func Register(c *sim.Codecs) error {
	return sim.Errors(
		c.RegisterCommand[*worldstate.State](
			WorldChatPrivateSendCommand{}, decodeWorldChatPrivateSendCommand,
		),
		c.RegisterCommand[*zonestate.State](
			ZoneCharacterEnterCommand{}, decodeZoneCharacterEnterCommand,
		),
		c.RegisterCommand[*zonestate.State](ZoneChatSendCommand{}, decodeZoneChatSendCommand),
		c.RegisterCommand[*zonelayerstate.State](
			ZoneLayerCharacterMoveCommand{}, decodeZoneLayerCharacterMoveCommand,
		),
		c.RegisterCommand[*zonelayerstate.State](
			ZoneLayerCharacterNormalAttackTriggerCommand{},
			decodeZoneLayerCharacterNormalAttackTriggerCommand,
		),
		c.RegisterCommand[*zonelayerstate.State](
			ZoneLayerChatSendCommand{}, decodeZoneLayerChatSendCommand,
		),
		c.RegisterCommand[*zonelayerstate.State](
			ZoneLayerCharacterExitToCommand{}, decodeZoneLayerCharacterExitToCommand,
		),
		c.RegisterCommand[*zonelayerstate.State](
			ZoneLayerCharacterItemPickupCommand{}, decodeZoneLayerCharacterItemPickupCommand,
		),
		c.RegisterCommand[*zonelayerstate.State](
			ZoneLayerCharacterShieldGrantCommand{}, decodeZoneLayerCharacterShieldGrantCommand,
		),
	)
}
