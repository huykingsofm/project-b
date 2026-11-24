package world

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
)

func TestAZoneLayerIsNotAddedWhenTheWorldHasNoNumberLeftForItsID(t *testing.T) {
	st := NewState(1)
	st.lastStateID = sim.MaxLocalStateID - 1
	id, err := st.AddZoneLayer()
	if err != nil || id.Local() != sim.MaxLocalStateID {
		t.Fatalf("the last number was refused: %v, %v", id, err)
	}
	if _, err := st.AddZoneLayer(); err == nil {
		t.Fatal("a zonelayer was added after the last number was given")
	}
	if got := len(st.ZoneLayerIDs()); got != 1 || st.lastStateID != sim.MaxLocalStateID {
		t.Fatalf("%d zonelayers and the last number %d after a refusal", got, st.lastStateID)
	}
}
