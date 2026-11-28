package world_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/world"
)

func TestTheWorldListsTheZoneAndTheZoneLayerThatTheClusterAnswersWithAndNoOther(t *testing.T) {
	st := world.NewState()
	zone := sim.NewStateID(state.ZoneType, 3)
	layer := sim.NewStateID(state.ZoneLayerType, 8)
	answer := func(tag byte, engine sim.StateID, failure string) {
		st.HandleDirective(nil, nil, sim.DirectiveResponse{
			ID: 1, Kind: sim.DirectiveCreateEngine, Tag: []byte{tag}, Engine: engine,
			Failure: failure,
		})
	}
	answer(world.TagZone, zone, "")
	answer(world.TagZoneLayer, layer, "")
	if got := st.ZoneIDs(); len(got) != 1 || got[0] != zone {
		t.Fatalf("the world lists the zones %v, want %v", got, zone)
	}
	if got := st.ZoneLayerIDs(); len(got) != 1 || got[0] != layer {
		t.Fatalf("the world lists the zonelayers %v, want %v", got, layer)
	}
	// an engine that was not made is not listed, and nor is the answer to something else
	answer(world.TagZone, sim.NewStateID(state.ZoneType, 4), "full")
	st.HandleDirective(nil, nil, sim.DirectiveResponse{
		Kind: sim.DirectiveRetireEngine, Tag: []byte{world.TagZone}, Engine: layer,
	})
	st.HandleDirective(nil, nil, sim.DirectiveResponse{Kind: sim.DirectiveCreateEngine})
	if len(st.ZoneIDs()) != 1 || len(st.ZoneLayerIDs()) != 1 {
		t.Fatalf("the world lists %v and %v", st.ZoneIDs(), st.ZoneLayerIDs())
	}
}
