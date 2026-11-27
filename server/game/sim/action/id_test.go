package action_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
)

// anActionOfEachKind has an action of every kind of the package.
func anActionOfEachKind() []sim.BaseAction {
	return []sim.BaseAction{
		action.WorldChatPrivateSendAction{}, action.WorldCharacterItemAddAction{},
		action.ZoneChatSendAction{}, action.ZoneCharacterEnterAction{},
		action.ZoneLayerChatSendAction{}, action.ZoneLayerCharacterEnterAction{},
		action.ZoneLayerCharacterMoveAction{},
		action.ZoneLayerCharacterNormalAttackTriggerAction{},
		action.ZoneLayerCharacterDamageAction{},
		action.ZoneLayerCharacterShieldGrantAction{},
		action.ZoneLayerCharacterRegenAction{}, action.ZoneLayerCharacterExitToAction{},
		action.ZoneLayerCharacterItemPickupAction{},
		action.ZoneLayerItemEntityAddAction{}, action.ZoneLayerItemEntityRemoveAction{},
		action.WorldZoneAddAction{}, action.WorldZoneLayerAddAction{},
		action.ZoneLayerAnnounceToZoneAction{}, action.ZoneZoneLayerAddAction{},
		action.ZoneExitAddAction{}, action.WorldZoneExitAddAction{},
	}
}

// registerSize is how many names the constant block of a file declares.
func registerSize(t *testing.T, file string) int {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, d := range f.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.CONST {
			for _, spec := range gd.Specs {
				n += len(spec.(*ast.ValueSpec).Names)
			}
		}
	}
	return n
}

func TestEveryActionReturnsItsOwnIDFromTheRegisterAndEveryNumberOfItIsUsed(t *testing.T) {
	var got []sim.ActionID
	for _, a := range anActionOfEachKind() {
		got = append(got, a.ID())
	}
	slices.Sort(got)
	n := registerSize(t, "id.go")
	if len(got) != n {
		t.Fatalf(
			"%d actions and %d IDs in the register: each action has an ID of its own and each ID "+
				"an action",
			len(got),
			n,
		)
	}
	for i, id := range got {
		if id != sim.ActionID(i+1) {
			t.Fatalf("the IDs of the actions are %v, want 1 to %d, each once", got, n)
		}
	}
}

func TestTheNumbersOfTheIDsAndTheCooldownKeysAreFixedForTheFormatOfTheLog(t *testing.T) {
	// A number that changes breaks every log that was written: add at the end, never renumber.
	if action.WorldChatPrivateSendActionID != 1 || action.ZoneLayerCharacterMoveActionID != 7 ||
		action.ZoneLayerItemEntityRemoveActionID != 15 || action.WorldZoneAddActionID != 16 ||
		action.ZoneZoneLayerAddActionID != 19 || action.WorldZoneExitAddActionID != 21 {
		t.Fatal("the number of an action ID changed")
	}
	if action.ZoneLayerCharacterMoveActionCooldownKey != 1 ||
		action.ZoneLayerCharacterNormalAttackTriggerActionCooldownKey != 2 {
		t.Fatal("the number of a cooldown key changed")
	}
	if registerSize(t, "cooldown_key.go") != 2 {
		t.Fatal("a cooldown key was added: add its number to this test")
	}
}
