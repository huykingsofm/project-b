package command

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
)

// EnterWorldZoneCommand asks to put a character in the world, in this zone.
type EnterWorldZoneCommand struct {
	Character zonelayerstate.Character
}

func (c EnterWorldZoneCommand) TranslateZone() []action.ZoneAction {
	return []action.ZoneAction{&action.ZoneCharacterEnterAction{Character: c.Character}}
}

// ChatZoneCommand asks to say something to the zone.
type ChatZoneCommand struct {
	FromID state.CharacterID
	Text   string
}

func (c ChatZoneCommand) TranslateZone() []action.ZoneAction {
	return []action.ZoneAction{&action.ZoneChatSendAction{FromID: c.FromID, Text: c.Text}}
}
