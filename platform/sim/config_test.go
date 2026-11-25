package sim

import (
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

func setMaxDepth(text string) []ConfigOverride {
	return []ConfigOverride{{Name: MaxDepth.Name(), Text: text}}
}

func TestANewStateHasEveryVariableAtItsDefault(t *testing.T) {
	s := NewConfigState()
	snap := s.Snapshot()
	if MaxDepth.Get(snap) != 4 {
		t.Fatalf("max depth %d", MaxDepth.Get(snap))
	}
	if s.ID() != ConfigStateID || s.Type() != ConfigStateType {
		t.Fatalf("the state is %d of the type %d", s.ID(), s.Type())
	}
}

func TestAChangeSetsAVariableAndAnUnsetPutsItBack(t *testing.T) {
	s := NewConfigState()
	if err := s.ApplyConfigChange(setMaxDepth("7"), nil); err != nil {
		t.Fatal(err)
	}
	if snap := s.Snapshot(); MaxDepth.Get(snap) != 7 {
		t.Fatalf("after a set: max depth %d", MaxDepth.Get(snap))
	}
	if err := s.ApplyConfigChange(nil, []string{MaxDepth.Name()}); err != nil {
		t.Fatal(err)
	}
	if snap := s.Snapshot(); MaxDepth.Get(snap) != 4 {
		t.Fatalf("after an unset: max depth %d", MaxDepth.Get(snap))
	}
}

func TestAChangeThatLeavesEveryVariableAsItWasMakesNoNewSnapshot(t *testing.T) {
	s := NewConfigState()
	_ = s.ApplyConfigChange(setMaxDepth("7"), nil)
	before := s.Snapshot()
	for name, c := range map[string]struct {
		set   []ConfigOverride
		unset []string
	}{
		"the same value":        {set: setMaxDepth("7")},
		"a variable at default": {unset: []string{MaxDepth.Name() + "-not"}},
		"a name that is none":   {set: []ConfigOverride{{Name: "no.such.variable", Text: "1"}}},
	} {
		if err := s.ApplyConfigChange(c.set, c.unset); err != nil || s.Snapshot() != before {
			t.Errorf("%s: a new snapshot was made (%v)", name, err)
		}
	}
}

func TestAValueThatIsWrongRefusesTheWholeChangeAndChangesNothing(t *testing.T) {
	s := NewConfigState()
	_ = s.ApplyConfigChange(setMaxDepth("7"), nil)
	hash := s.Hash()
	for _, text := range []string{`"seven"`, "99", "2.5", ""} {
		if err := s.ApplyConfigChange(setMaxDepth(text), nil); err == nil {
			t.Errorf("%q was taken", text)
		}
	}
	if s.Hash() != hash || MaxDepth.Get(s.Snapshot()) != 7 {
		t.Fatal("a refused change changed the state")
	}
}

func TestACloneIsNotChangedByTheStateItCameFromAndTheOtherWayRound(t *testing.T) {
	s := NewConfigState()
	_ = s.ApplyConfigChange(setMaxDepth("7"), nil)
	clone := s.Clone().(*ConfigState)
	_ = s.ApplyConfigChange(setMaxDepth("9"), nil)
	if MaxDepth.Get(clone.Snapshot()) != 7 {
		t.Fatal("the clone followed the state")
	}
	_ = clone.ApplyConfigChange(setMaxDepth("3"), nil)
	if MaxDepth.Get(s.Snapshot()) != 9 {
		t.Fatal("the state followed the clone")
	}
}

func TestAStateWrittenAndReadBackHasTheSameHashAndTheSameConfiguration(t *testing.T) {
	s := NewConfigState()
	s.NextTick()
	_ = s.ApplyConfigChange(setMaxDepth("7"), nil)
	var w binary.Writer
	if err := s.Encode(&w); err != nil {
		t.Fatal(err)
	}
	got, err := DecodeConfigState(binary.NewReader(w.B), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hash() != s.Hash() || got.Tick() != 1 || MaxDepth.Get(got.Snapshot()) != 7 {
		t.Fatal("the state that was read back differs")
	}
	for n := range len(w.B) {
		if _, err := DecodeConfigState(binary.NewReader(w.B[:n]), 1); err == nil {
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
	s := NewConfigState()
	wanted := wantedConfig(t, map[string]string{MaxDepth.Name(): "7", "no.such.variable": "1"})
	cmd, changed := NewUpdateConfigCommand(s.Snapshot(), wanted)
	if !changed || !reflect.DeepEqual(cmd.Set, setMaxDepth("7")) || len(cmd.Unset) != 0 {
		t.Fatalf("command %+v, changed %v", cmd, changed)
	}
	_ = s.ApplyConfigChange(cmd.Set, cmd.Unset)
	if _, changed := NewUpdateConfigCommand(s.Snapshot(), wanted); changed {
		t.Fatal("the same snapshot made a command")
	}
	cmd, changed = NewUpdateConfigCommand(s.Snapshot(), config.Defaults())
	if !changed || !reflect.DeepEqual(cmd.Unset, []string{MaxDepth.Name()}) {
		t.Fatalf("the defaults: %+v, changed %v", cmd, changed)
	}
}

func TestTheCommandAndTheActionAreWrittenAndReadBackAndTheActionChangesTheState(t *testing.T) {
	codecs := NewCodecs()
	cmd := UpdateConfigCommand{Set: setMaxDepth("7"), Unset: []string{"x.y"}}
	var w binary.Writer
	if err := EncodeCommand(&w, cmd); err != nil {
		t.Fatal(err)
	}
	got, err := codecs.DecodeCommand(binary.NewReader(w.B), 1)
	if err != nil || !reflect.DeepEqual(got, cmd) {
		t.Fatalf("command read back as %+v (%v)", got, err)
	}
	actions := cmd.Translate()
	w = binary.Writer{}
	if err := EncodeAction(&w, actions[0]); err != nil {
		t.Fatal(err)
	}
	read, err := codecs.DecodeAction(binary.NewReader(w.B), 1)
	if err != nil || !reflect.DeepEqual(read, actions[0]) {
		t.Fatalf("action read back as %+v (%v)", read, err)
	}
	s := NewConfigState()
	actions[0].Apply(&Context{}, s, &Registry[*ConfigState]{}, nil)
	if MaxDepth.Get(s.Snapshot()) != 7 {
		t.Fatal("the action did not set the variable")
	}
}
