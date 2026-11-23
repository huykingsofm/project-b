package command

import (
	"github.com/huykingsofm/project-b/platform/sim"
)

// Register registers every command of the game with the codecs, with the type of state it is for:
// a command that is read back is a command of that state and of no other. A number or a type that
// two commands share is an error.
func Register(c *sim.Codecs) error {
	return sim.Errors(
		c.RegisterCommand(decodeWorldChatPrivateSendCommand),
		c.RegisterCommand(decodeZoneCharacterEnterCommand),
		c.RegisterCommand(decodeZoneChatSendCommand),
		c.RegisterCommand(decodeZoneLayerCharacterMoveCommand),
		c.RegisterCommand(decodeZoneLayerCharacterNormalAttackTriggerCommand),
		c.RegisterCommand(decodeZoneLayerChatSendCommand),
		c.RegisterCommand(decodeZoneLayerCharacterExitToCommand),
		c.RegisterCommand(decodeZoneLayerCharacterItemPickupCommand),
		c.RegisterCommand(decodeZoneLayerCharacterShieldGrantCommand),
	)
}
