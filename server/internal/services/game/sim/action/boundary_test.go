package action

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/shared/aliasing"
)

// allActions has a value of every action of the package, for the tests that look at all of them.
var allActions = []any{
	WorldChatPrivateSendAction{}, WorldCharacterItemAddAction{},
	ZoneChatSendAction{}, ZoneCharacterEnterAction{},
	ZoneLayerChatSendAction{}, ZoneLayerCharacterEnterAction{}, ZoneLayerCharacterMoveAction{},
	ZoneLayerCharacterNormalAttackTriggerAction{}, ZoneLayerCharacterDamageAction{},
	ZoneLayerCharacterShieldGrantAction{},
	ZoneLayerCharacterRegenAction{}, ZoneLayerCharacterExitToAction{},
	ZoneLayerCharacterItemPickupAction{},
	ZoneLayerItemEntityAddAction{}, ZoneLayerItemEntityRemoveAction{},
}

func TestAllActionsListsEveryActionOfThePackage(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(
		fset,
		".",
		func(fi fs.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") },
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	var declared []string
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, d := range f.Decls {
				gd, ok := d.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, spec := range gd.Specs {
					ts := spec.(*ast.TypeSpec)
					if _, ok := ts.Type.(*ast.StructType); ok &&
						strings.HasSuffix(ts.Name.Name, "Action") {
						declared = append(declared, ts.Name.Name)
					}
				}
			}
		}
	}
	var listed []string
	for _, a := range allActions {
		listed = append(listed, reflect.TypeOf(a).Name())
	}
	slices.Sort(declared)
	slices.Sort(listed)
	if !slices.Equal(declared, listed) {
		t.Fatalf(
			"the actions declared are\n%v\nand the ones in allActions are\n%v", declared, listed,
		)
	}
}

// An action is made by one engine and may be put in a message for another, so what it holds is a
// value or a type with Clone, and whoever stores it in a state copies it (R29).
func TestEveryActionHoldsOnlyValuesOrTypesWithClone(t *testing.T) {
	rules := aliasing.Rules{Boundary: true}
	for _, a := range allActions {
		if err := aliasing.CheckCloneable(reflect.TypeOf(a), rules); err != nil {
			t.Errorf("%T: %v", a, err)
		}
	}
}
