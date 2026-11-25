package command

import (
	"github.com/huykingsofm/project-b/platform/sim"
)

// The IDs of the commands: the register of the kinds of command. The numbers count up from 1 and 0
// is no command. A new command takes the next number, at the end of the list, and a number is never
// changed or reused, because the event log writes it. Each command returns its own from its method
// ID().
const (
	WorldChatPrivateSendCommandID sim.CommandID = iota + 1
	ZoneCharacterEnterCommandID
	ZoneChatSendCommandID
	ZoneLayerCharacterMoveCommandID
	ZoneLayerCharacterNormalAttackTriggerCommandID
	ZoneLayerChatSendCommandID
	ZoneLayerCharacterExitToCommandID
	ZoneLayerCharacterItemPickupCommandID
	ZoneLayerCharacterShieldGrantCommandID
)
