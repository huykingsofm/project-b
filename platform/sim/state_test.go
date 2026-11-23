package sim_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
)

func TestAStateTypeHasOneNameAndANameBelongsToOneType(t *testing.T) {
	if err := sim.MapStateTypeToName(201, "naming test"); err != nil {
		t.Fatal(err)
	}
	if got := sim.StateType(201).String(); got != "naming test" {
		t.Fatalf("String = %q", got)
	}
	for name, call := range map[string]func() error{
		"a second name":     func() error { return sim.MapStateTypeToName(201, "another") },
		"the same name too": func() error { return sim.MapStateTypeToName(201, "naming test") },
		"a name in use":     func() error { return sim.MapStateTypeToName(202, "naming test") },
		"no name":           func() error { return sim.MapStateTypeToName(203, "") },
	} {
		if err := call(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if got := sim.StateType(204).String(); got != "state type 204" {
		t.Fatalf("a type with no name is called %q", got)
	}
}
