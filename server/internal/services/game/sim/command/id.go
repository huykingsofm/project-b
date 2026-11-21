package command

// The IDs of the commands: the register of the kinds of command. The numbers count up from 1 and 0 is no command. A
// new command takes the next number, at the end of the list, and a number is never changed or reused, because the
// replay log writes it. Each command returns its own from its method ID().
const (
	WorldChatPrivateSendCommandID ID = iota + 1
	ZoneCharacterEnterCommandID
	ZoneChatSendCommandID
	ZoneLayerCharacterMoveCommandID
	ZoneLayerCharacterNormalAttackTriggerCommandID
	ZoneLayerChatSendCommandID
	ZoneLayerCharacterExitToCommandID
	ZoneLayerCharacterItemPickupCommandID
	ZoneLayerCharacterShieldGrantCommandID
)
