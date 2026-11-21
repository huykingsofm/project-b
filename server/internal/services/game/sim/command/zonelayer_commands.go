package command

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
)

// MoveZoneLayerCommand asks to hold a direction for a number of ticks.
type MoveZoneLayerCommand struct {
	CharacterID state.CharacterID
	Dir         state.Dir
	Ticks       uint32
}

func (c MoveZoneLayerCommand) TranslateZoneLayer() []action.ZoneLayerAction {
	return []action.ZoneLayerAction{&action.ZoneLayerCharacterMoveAction{CharacterID: c.CharacterID, Dir: c.Dir, Ticks: c.Ticks}}
}

// NormalAttackZoneLayerCommand asks to hit a target character with the normal attack.
type NormalAttackZoneLayerCommand struct {
	CharacterID state.CharacterID
	TargetID    state.CharacterID
}

func (c NormalAttackZoneLayerCommand) TranslateZoneLayer() []action.ZoneLayerAction {
	return []action.ZoneLayerAction{&action.ZoneLayerCharacterNormalAttackAction{CharacterID: c.CharacterID, TargetID: c.TargetID}}
}

// ChatZoneLayerCommand asks to say something to the zonelayer the character is in.
type ChatZoneLayerCommand struct {
	FromID state.CharacterID
	Text   string
}

func (c ChatZoneLayerCommand) TranslateZoneLayer() []action.ZoneLayerAction {
	return []action.ZoneLayerAction{&action.ZoneLayerChatSendAction{FromID: c.FromID, Text: c.Text}}
}

// UseExitZoneLayerCommand asks to take the exit, an entity of the zonelayer, that the character stands in.
type UseExitZoneLayerCommand struct {
	CharacterID  state.CharacterID
	ExitEntityID state.EntityID
}

// PickupItemZoneLayerCommand asks to pick up an item that lies on the ground.
type PickupItemZoneLayerCommand struct {
	CharacterID  state.CharacterID
	ItemEntityID state.EntityID
}

func (c PickupItemZoneLayerCommand) TranslateZoneLayer() []action.ZoneLayerAction {
	return []action.ZoneLayerAction{&action.ZoneLayerCharacterItemPickupAction{CharacterID: c.CharacterID, ItemEntityID: c.ItemEntityID}}
}

func (c UseExitZoneLayerCommand) TranslateZoneLayer() []action.ZoneLayerAction {
	return []action.ZoneLayerAction{&action.ZoneLayerCharacterExitToAction{CharacterID: c.CharacterID, ExitEntityID: c.ExitEntityID}}
}

// GrantShieldZoneLayerCommand gives a character a shield. A skill or an item sends it in a game; here
// it is the command that stands for them.
type GrantShieldZoneLayerCommand struct {
	CharacterID state.CharacterID
	Duration    uint64
}

func (c GrantShieldZoneLayerCommand) TranslateZoneLayer() []action.ZoneLayerAction {
	return []action.ZoneLayerAction{&action.ZoneLayerCharacterShieldAction{CharacterID: c.CharacterID, Duration: c.Duration}}
}
