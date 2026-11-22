package aliasing_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/shared/aliasing"
)

type inner struct{ Items []int }

type outer struct {
	Name   string
	M      map[string]int
	P      *inner
	hidden []int // unexported: it is looked at too
}

func TestDisjointFindsAMapAPointerAndASliceThatTwoValuesShare(t *testing.T) {
	a := outer{M: map[string]int{"x": 1}, P: &inner{Items: []int{1}}, hidden: []int{1, 2, 3}}
	for name, b := range map[string]outer{
		"a map":                       {M: a.M},
		"a pointer":                   {P: a.P},
		"a slice that is unexported":  {hidden: a.hidden},
		"a part of the same array":    {hidden: a.hidden[1:2]},
		"the room behind a short one": {hidden: a.hidden[:0]},
		"a slice inside a pointer":    {P: &inner{Items: a.P.Items}},
	} {
		if err := aliasing.Disjoint(a, b); err == nil {
			t.Errorf("%s: shared memory was not found", name)
		}
	}
}

func TestDisjointAcceptsValuesThatShareNothingAndStringsAndNil(t *testing.T) {
	a := outer{
		Name: "same", M: map[string]int{"x": 1}, P: &inner{Items: []int{1}}, hidden: []int{1},
	}
	b := outer{
		Name: "same", M: map[string]int{"x": 1}, P: &inner{Items: []int{1}}, hidden: []int{1},
	}
	if err := aliasing.Disjoint(a, b); err != nil {
		t.Fatalf("separate copies: %v", err)
	}
	if err := aliasing.Disjoint(outer{}, outer{}); err != nil {
		t.Fatalf("nil: %v", err)
	}
	if err := aliasing.Disjoint(&a, &b); err != nil {
		t.Fatalf("pointers to separate copies: %v", err)
	}
	if err := aliasing.Disjoint(&a, &a); err == nil {
		t.Fatal("a value shares its memory with itself")
	}
}

func TestDisjointFollowsInterfacesAndMapValuesAndCycles(t *testing.T) {
	shared := &inner{Items: []int{1}}
	a := map[string]any{"k": []any{shared}}
	b := map[string][]*inner{"other": {shared}}
	if err := aliasing.Disjoint(a, b); err == nil {
		t.Fatal("a pointer in an interface in a slice in a map was not found")
	}
	type node struct{ Next *node }
	n := &node{}
	n.Next = n
	if err := aliasing.Disjoint(n, &node{}); err != nil {
		t.Fatalf("a cycle: %v", err)
	}
}

// listWithClone holds a slice and copies it.
type listWithClone struct{ Items []int }

func (l listWithClone) Clone() listWithClone { return listWithClone{Items: slices.Clone(l.Items)} }

type holder struct {
	Count int
	List  listWithClone
}

type badHolder struct {
	Count int
	Items []int
}

// a slice of a type with a slice and no Clone, inside a type with Clone
type badInside struct{ Rows []badHolder }

func (b badInside) Clone() badInside { return b }

type withChannel struct{ C chan int }

func (w withChannel) Clone() withChannel { return w }

type withInterface struct{ V any }

func (w withInterface) Clone() withInterface { return w }

type keyed struct{ M map[any]int }

func (k keyed) Clone() keyed { return k }

func check(t reflect.Type, rules aliasing.Rules) error { return aliasing.CheckCloneable(t, rules) }

func TestCheckCloneableAcceptsValuesAndTypesWithClone(t *testing.T) {
	type pureStruct struct {
		A int
		B [3]uint8
		S string
		P struct{ X, Y int32 }
	}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[int](),
		reflect.TypeFor[pureStruct](),
		reflect.TypeFor[listWithClone](),
		reflect.TypeFor[keyed](),
	} {
		if err := check(typ, aliasing.Rules{}); err != nil {
			t.Errorf("%v: %v", typ, err)
		}
	}
}

func TestCheckCloneableRefusesAReferenceWithNoCloneAndSaysWhere(t *testing.T) {
	err := check(reflect.TypeFor[badHolder](), aliasing.Rules{})
	if err == nil || !strings.Contains(err.Error(), "no Clone") {
		t.Fatalf("a struct with a slice and no Clone: %v", err)
	}
	if err := check(reflect.TypeFor[holder](), aliasing.Rules{}); err == nil {
		t.Fatal(
			"a struct that holds a type with Clone is not itself copied right by assignment, so " +
				"it needs its own Clone",
		)
	}
	err = check(reflect.TypeFor[badInside](), aliasing.Rules{})
	if err == nil || !strings.Contains(err.Error(), "Rows") {
		t.Fatalf("an element with no Clone inside a type with Clone: %v", err)
	}
}

func TestCheckCloneableRefusesChannelsFunctionsAndInterfacesThatAreNotAllowed(t *testing.T) {
	if err := check(reflect.TypeFor[withChannel](), aliasing.Rules{}); err == nil {
		t.Error("a channel was accepted")
	}
	if err := check(reflect.TypeFor[withInterface](), aliasing.Rules{}); err == nil {
		t.Error("an interface that was not allowed was accepted")
	}
	if err := check(
		reflect.TypeFor[withInterface](),
		aliasing.Rules{Interfaces: []reflect.Type{reflect.TypeFor[any]()}},
	); err != nil {
		t.Errorf("an interface that was allowed: %v", err)
	}
}

type sharedOnPurpose struct{ Big []int }

type usesShared struct{ S *sharedOnPurpose }

func (u usesShared) Clone() usesShared { return u } // shares S, which never changes

type withEvents struct {
	Count  int
	events []any
}

func (w withEvents) Clone() withEvents { return withEvents{Count: w.Count} }

func TestCheckCloneableAllowsSharedTypesAndSkipsFieldsThatAreNotData(t *testing.T) {
	if err := check(reflect.TypeFor[usesShared](), aliasing.Rules{}); err == nil {
		t.Error("a pointer to a type that is shared was accepted without saying so")
	}
	shared := aliasing.Rules{Shared: []reflect.Type{reflect.TypeFor[*sharedOnPurpose]()}}
	if err := check(reflect.TypeFor[usesShared](), shared); err != nil {
		t.Errorf("a shared type that was allowed: %v", err)
	}
	if err := check(reflect.TypeFor[withEvents](), aliasing.Rules{}); err == nil {
		t.Error("an interface that holds events was accepted")
	}
	skip := aliasing.Rules{Skip: []string{"aliasing_test.withEvents.events"}}
	if err := check(reflect.TypeFor[withEvents](), skip); err != nil {
		t.Errorf("a field that is skipped: %v", err)
	}
}

type carries struct {
	Count int
	List  listWithClone
}

func TestBoundaryAcceptsAStructOfValuesAndTypesWithCloneThatHasNoCloneItself(t *testing.T) {
	if err := check(reflect.TypeFor[carries](), aliasing.Rules{}); err == nil {
		t.Error("without Boundary a struct that holds a type with Clone needs a Clone of its own")
	}
	if err := check(reflect.TypeFor[carries](), aliasing.Rules{Boundary: true}); err != nil {
		t.Errorf("with Boundary: %v", err)
	}
	if err := check(reflect.TypeFor[badHolder](), aliasing.Rules{Boundary: true}); err == nil {
		t.Error("with Boundary a slice that is not in a type with Clone was accepted")
	}
}
