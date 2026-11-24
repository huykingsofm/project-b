package sim_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
)

func TestAStateTypeHasOneNameAndANameBelongsToOneType(t *testing.T) {
	sim.MapStateTypeToName(201, "naming test")
	if got := sim.StateType(201).String(); got != "naming test" {
		t.Fatalf("String = %q", got)
	}
	for name, call := range map[string]func(){
		"a second name":     func() { sim.MapStateTypeToName(201, "another") },
		"the same name too": func() { sim.MapStateTypeToName(201, "naming test") },
		"a name in use":     func() { sim.MapStateTypeToName(202, "naming test") },
		"no name":           func() { sim.MapStateTypeToName(203, "") },
	} {
		if !panics(call) {
			t.Errorf("%s was accepted", name)
		}
	}
	if got := sim.StateType(204).String(); got != "state type 204" {
		t.Fatalf("a type with no name is called %q", got)
	}
}

// panics reports whether a call panics.
func panics(call func()) (did bool) {
	defer func() { did = recover() != nil }()
	call()
	return false
}
