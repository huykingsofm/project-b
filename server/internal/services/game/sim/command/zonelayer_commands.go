package command

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
)

// MoveZoneLayerCommand asks to hold a direction for a number of ticks.
type MoveZoneLayerCommand struct {
	CharID state.CharacterID
	Dir    state.Dir
	Ticks  uint32
}

func (c MoveZoneLayerCommand) TranslateZoneLayer() []action.ZoneLayerAction {
	return []action.ZoneLayerAction{&action.ZoneLayerCharacterMoveAction{CharID: c.CharID, Dir: c.Dir, Ticks: c.Ticks}}
}

// NormalAttackZoneLayerCommand asks to hit a target character with the normal attack.
type NormalAttackZoneLayerCommand struct {
	CharID   state.CharacterID
	TargetID state.CharacterID
}

func (c NormalAttackZoneLayerCommand) TranslateZoneLayer() []action.ZoneLayerAction {
	return []action.ZoneLayerAction{&action.ZoneLayerCharacterNormalAttackAction{CharID: c.CharID, TargetID: c.TargetID}}
}

// ChatZoneLayerCommand asks to say something to the zonelayer the character is in.
type ChatZoneLayerCommand struct {
	FromID state.CharacterID
	Text   string
}

func (c ChatZoneLayerCommand) TranslateZoneLayer() []action.ZoneLayerAction {
	return []action.ZoneLayerAction{&action.ZoneLayerChatSendAction{FromID: c.FromID, Text: c.Text}}
}

// UseExitZoneLayerCommand asks to take the exit the character stands in.
type UseExitZoneLayerCommand struct {
	CharID state.CharacterID
	ExitID state.ExitID
}

func (c UseExitZoneLayerCommand) TranslateZoneLayer() []action.ZoneLayerAction {
	return []action.ZoneLayerAction{&action.ZoneLayerCharacterExitToAction{CharID: c.CharID, ExitID: c.ExitID}}
}

// GrantShieldZoneLayerCommand gives a character a shield. A skill or an item sends it in a game; here
// it is the command that stands for them.
type GrantShieldZoneLayerCommand struct {
	CharID   state.CharacterID
	Duration uint64
}

func (c GrantShieldZoneLayerCommand) TranslateZoneLayer() []action.ZoneLayerAction {
	return []action.ZoneLayerAction{&action.ZoneLayerCharacterShieldAction{CharID: c.CharID, Duration: c.Duration}}
}
