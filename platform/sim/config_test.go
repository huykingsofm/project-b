package sim_test

import (
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

func setMaxDepth(text string) []sim.ConfigOverride {
	return []sim.ConfigOverride{{Name: sim.MaxDepth.Name(), Text: text}}
}

func TestANewStateHasEveryVariableAtItsDefault(t *testing.T) {
	s := sim.NewConfigState()
	snap := s.Snapshot()
	if sim.MaxDepth.Get(snap) != 4 {
		t.Fatalf("max depth %d", sim.MaxDepth.Get(snap))
	}
	if s.Type() != sim.ConfigStateType {
		t.Fatalf("the state is of the type %d", s.Type())
	}
}

func TestAChangeSetsAVariableAndAnUnsetPutsItBack(t *testing.T) {
	s := sim.NewConfigState()
	if err := s.ApplyConfigChange(setMaxDepth("7"), nil); err != nil {
		t.Fatal(err)
	}
	if snap := s.Snapshot(); sim.MaxDepth.Get(snap) != 7 {
		t.Fatalf("after a set: max depth %d", sim.MaxDepth.Get(snap))
	}
	if err := s.ApplyConfigChange(nil, []string{sim.MaxDepth.Name()}); err != nil {
		t.Fatal(err)
	}
	if snap := s.Snapshot(); sim.MaxDepth.Get(snap) != 4 {
		t.Fatalf("after an unset: max depth %d", sim.MaxDepth.Get(snap))
	}
}

func TestAChangeThatLeavesEveryVariableAsItWasMakesNoNewSnapshot(t *testing.T) {
	s := sim.NewConfigState()
	_ = s.ApplyConfigChange(setMaxDepth("7"), nil)
	before := s.Snapshot()
	for name, c := range map[string]struct {
		set   []sim.ConfigOverride
		unset []string
	}{
		"the same value":        {set: setMaxDepth("7")},
		"a variable at default": {unset: []string{sim.MaxDepth.Name() + "-not"}},
		"a name that is none":   {set: []sim.ConfigOverride{{Name: "no.such.variable", Text: "1"}}},
	} {
		if err := s.ApplyConfigChange(c.set, c.unset); err != nil || s.Snapshot() != before {
			t.Errorf("%s: a new snapshot was made (%v)", name, err)
		}
	}
}

func TestAValueThatIsWrongRefusesTheWholeChangeAndChangesNothing(t *testing.T) {
	s := sim.NewConfigState()
	_ = s.ApplyConfigChange(setMaxDepth("7"), nil)
	hash := s.Hash()
	for _, text := range []string{`"seven"`, "99", "2.5", ""} {
		if err := s.ApplyConfigChange(setMaxDepth(text), nil); err == nil {
			t.Errorf("%q was taken", text)
		}
	}
	if s.Hash() != hash || sim.MaxDepth.Get(s.Snapshot()) != 7 {
		t.Fatal("a refused change changed the state")
	}
}

func TestACloneIsNotChangedByTheStateItCameFromAndTheOtherWayRound(t *testing.T) {
	s := sim.NewConfigState()
	_ = s.ApplyConfigChange(setMaxDepth("7"), nil)
	clone := s.Clone().(*sim.ConfigState)
	_ = s.ApplyConfigChange(setMaxDepth("9"), nil)
	if sim.MaxDepth.Get(clone.Snapshot()) != 7 {
		t.Fatal("the clone followed the state")
	}
	_ = clone.ApplyConfigChange(setMaxDepth("3"), nil)
	if sim.MaxDepth.Get(s.Snapshot()) != 9 {
		t.Fatal("the state followed the clone")
	}
}

func TestAStateWrittenAndReadBackHasTheSameHashAndTheSameConfiguration(t *testing.T) {
	s := sim.NewConfigState()
	s.NextTick()
	_ = s.ApplyConfigChange(setMaxDepth("7"), nil)
	var w binary.Writer
	if err := s.Encode(&w); err != nil {
		t.Fatal(err)
	}
	got, err := sim.DecodeConfigState(binary.NewReader(w.B), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hash() != s.Hash() || got.Tick() != 1 || sim.MaxDepth.Get(got.Snapshot()) != 7 {
		t.Fatal("the state that was read back differs")
	}
	for n := range len(w.B) {
		if _, err := sim.DecodeConfigState(binary.NewReader(w.B[:n]), 1); err == nil {
			t.Fatalf("a state cut to %d of %d bytes was read", n, len(w.B))
		}
	}
}

func wantedConfig(t *testing.T, document map[string]string) *config.Snapshot {
	t.Helper()
	snap, err := config.Build(document)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func TestACommandMakesTheChangeToTheWantedSnapshotAndSaysNothingWhenNothingChanges(t *testing.T) {
	s := sim.NewConfigState()
	wanted := wantedConfig(t, map[string]string{sim.MaxDepth.Name(): "7", "no.such.variable": "1"})
	cmd, changed := sim.NewUpdateConfigCommand(s.Snapshot(), wanted)
	if !changed || !reflect.DeepEqual(cmd.Set, setMaxDepth("7")) || len(cmd.Unset) != 0 {
		t.Fatalf("command %+v, changed %v", cmd, changed)
	}
	_ = s.ApplyConfigChange(cmd.Set, cmd.Unset)
	if _, changed := sim.NewUpdateConfigCommand(s.Snapshot(), wanted); changed {
		t.Fatal("the same snapshot made a command")
	}
	cmd, changed = sim.NewUpdateConfigCommand(s.Snapshot(), config.Defaults())
	if !changed || !reflect.DeepEqual(cmd.Unset, []string{sim.MaxDepth.Name()}) {
		t.Fatalf("the defaults: %+v, changed %v", cmd, changed)
	}
}

func TestTheCommandAndTheActionAreWrittenAndReadBackAndTheActionChangesTheState(t *testing.T) {
	codecs := sim.NewCodecs()
	cmd := sim.UpdateConfigCommand{Set: setMaxDepth("7"), Unset: []string{"x.y"}}
	var w binary.Writer
	if err := sim.EncodeCommand(&w, cmd); err != nil {
		t.Fatal(err)
	}
	got, err := codecs.DecodeCommand(binary.NewReader(w.B), 1)
	if err != nil || !reflect.DeepEqual(got, cmd) {
		t.Fatalf("command read back as %+v (%v)", got, err)
	}
	actions := cmd.Translate()
	w = binary.Writer{}
	if err := sim.EncodeAction(&w, actions[0]); err != nil {
		t.Fatal(err)
	}
	read, err := codecs.DecodeAction(binary.NewReader(w.B), 1)
	if err != nil || !reflect.DeepEqual(read, actions[0]) {
		t.Fatalf("action read back as %+v (%v)", read, err)
	}
	s := sim.NewConfigState()
	actions[0].Apply(&sim.Context{}, s, &sim.Registry[*sim.ConfigState]{}, nil)
	if sim.MaxDepth.Get(s.Snapshot()) != 7 {
		t.Fatal("the action did not set the variable")
	}
}
