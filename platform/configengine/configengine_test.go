package configengine

import (
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

func setMaxDepth(text string) []Override {
	return []Override{{Name: sim.MaxDepth.Name(), Text: text}}
}

func TestANewStateHasEveryVariableAtItsDefaultAtVersionZero(t *testing.T) {
	s := NewState()
	snap := s.Snapshot()
	if snap.Version() != 0 || sim.MaxDepth.Get(snap) != 4 {
		t.Fatalf("version %d, max depth %d", snap.Version(), sim.MaxDepth.Get(snap))
	}
	if s.ID() != sim.ConfigStateID || s.Type() != sim.ConfigStateType {
		t.Fatalf("the state is %d of the type %d", s.ID(), s.Type())
	}
}

func TestAChangeSetsAVariableAndAnUnsetPutsItBackAndEachMakesAVersion(t *testing.T) {
	s := NewState()
	if err := s.ApplyConfigChange(setMaxDepth("7"), nil); err != nil {
		t.Fatal(err)
	}
	if snap := s.Snapshot(); snap.Version() != 1 || sim.MaxDepth.Get(snap) != 7 {
		t.Fatalf("after a set: version %d, max depth %d", snap.Version(), sim.MaxDepth.Get(snap))
	}
	if err := s.ApplyConfigChange(nil, []string{sim.MaxDepth.Name()}); err != nil {
		t.Fatal(err)
	}
	if snap := s.Snapshot(); snap.Version() != 2 || sim.MaxDepth.Get(snap) != 4 {
		t.Fatalf("after an unset: version %d, max depth %d", snap.Version(), sim.MaxDepth.Get(snap))
	}
}

func TestAChangeThatLeavesEveryVariableAsItWasMakesNoVersion(t *testing.T) {
	s := NewState()
	_ = s.ApplyConfigChange(setMaxDepth("7"), nil)
	for name, c := range map[string]struct {
		set   []Override
		unset []string
	}{
		"the same value":        {set: setMaxDepth("7")},
		"a variable at default": {unset: []string{sim.MaxDepth.Name() + "-not"}},
		"a name that is none":   {set: []Override{{Name: "no.such.variable", Text: "1"}}},
	} {
		if err := s.ApplyConfigChange(c.set, c.unset); err != nil || s.Snapshot().Version() != 1 {
			t.Errorf("%s: version %d (%v), want 1", name, s.Snapshot().Version(), err)
		}
	}
}

func TestAValueThatIsWrongRefusesTheWholeChangeAndChangesNothing(t *testing.T) {
	s := NewState()
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
	s := NewState()
	_ = s.ApplyConfigChange(setMaxDepth("7"), nil)
	clone := s.Clone().(*State)
	_ = s.ApplyConfigChange(setMaxDepth("9"), nil)
	if sim.MaxDepth.Get(clone.Snapshot()) != 7 || clone.Snapshot().Version() != 1 {
		t.Fatal("the clone followed the state")
	}
	_ = clone.ApplyConfigChange(setMaxDepth("3"), nil)
	if sim.MaxDepth.Get(s.Snapshot()) != 9 {
		t.Fatal("the state followed the clone")
	}
}

func TestAStateWrittenAndReadBackHasTheSameHashAndTheSameConfiguration(t *testing.T) {
	s := NewState()
	s.NextTick()
	_ = s.ApplyConfigChange(setMaxDepth("7"), nil)
	var w binary.Writer
	if err := s.Encode(&w); err != nil {
		t.Fatal(err)
	}
	got, err := DecodeState(binary.NewReader(w.B), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hash() != s.Hash() || got.Tick() != 1 || sim.MaxDepth.Get(got.Snapshot()) != 7 ||
		got.Snapshot().Version() != 1 {
		t.Fatal("the state that was read back differs")
	}
	for n := range len(w.B) {
		if _, err := DecodeState(binary.NewReader(w.B[:n]), 1); err == nil {
			t.Fatalf("a state cut to %d of %d bytes was read", n, len(w.B))
		}
	}
}

func TestACommandMakesTheChangeFromTheDocumentAndSaysNothingWhenNothingChanges(t *testing.T) {
	s := NewState()
	doc := map[string]string{sim.MaxDepth.Name(): "7", "no.such.variable": "1"}
	cmd, changed, err := NewUpdateConfigCommand(s.Snapshot(), doc)
	if err != nil || !changed || !reflect.DeepEqual(cmd.Set, setMaxDepth("7")) ||
		len(cmd.Unset) != 0 {
		t.Fatalf("command %+v, changed %v, err %v", cmd, changed, err)
	}
	_ = s.ApplyConfigChange(cmd.Set, cmd.Unset)
	if _, changed, err := NewUpdateConfigCommand(s.Snapshot(), doc); changed || err != nil {
		t.Fatalf("the same document: changed %v, err %v", changed, err)
	}
	cmd, changed, err = NewUpdateConfigCommand(s.Snapshot(), map[string]string{})
	if err != nil || !changed || !reflect.DeepEqual(cmd.Unset, []string{sim.MaxDepth.Name()}) {
		t.Fatalf("an empty document: %+v, changed %v, err %v", cmd, changed, err)
	}
	tooDeep := map[string]string{sim.MaxDepth.Name(): "99"}
	if _, _, err := NewUpdateConfigCommand(nil, tooDeep); err == nil {
		t.Fatal("a document with a wrong value was taken")
	}
}

func TestTheCommandAndTheActionAreWrittenAndReadBackAndTheActionChangesTheState(t *testing.T) {
	codecs := sim.NewCodecs()
	if err := Register(codecs); err != nil {
		t.Fatal(err)
	}
	cmd := UpdateConfigCommand{Set: setMaxDepth("7"), Unset: []string{"x.y"}}
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
	s := NewState()
	actions[0].Apply(&sim.Context{}, s, &sim.Registry[*State]{}, nil)
	if sim.MaxDepth.Get(s.Snapshot()) != 7 {
		t.Fatal("the action did not set the variable")
	}
}
