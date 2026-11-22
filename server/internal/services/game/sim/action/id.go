package action

// The IDs of the actions: the register of the kinds of action. The numbers count up from 1 and 0 is
// no action. A new action takes the next number, at the end of the list, and a number is never
// changed or reused, because the replay log writes it. Each action returns its own from its method
// ID().
const (
	WorldChatPrivateSendActionID ID = iota + 1
	WorldCharacterItemAddActionID
	ZoneChatSendActionID
	ZoneCharacterEnterActionID
	ZoneLayerChatSendActionID
	ZoneLayerCharacterEnterActionID
	ZoneLayerCharacterMoveActionID
	ZoneLayerCharacterNormalAttackTriggerActionID
	ZoneLayerCharacterDamageActionID
	ZoneLayerCharacterShieldGrantActionID
	ZoneLayerCharacterRegenActionID
	ZoneLayerCharacterExitToActionID
	ZoneLayerCharacterItemPickupActionID
	ZoneLayerItemEntityAddActionID
	ZoneLayerItemEntityRemoveActionID
)
