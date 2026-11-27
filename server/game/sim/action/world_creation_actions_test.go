package action_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
)

const worldEngineID = sim.StateID(1<<56 | 1) // the first world: its type is 1

func TestTheWorldAsksForAZoneOfItsOwnOnTheMapAndForAZoneLayerThatStartsByTellingItsZone(
	t *testing.T,
) {
	tiles, err := tilemap.New(4, 3, tilemap.Tile{X: 1, Y: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var reg sim.Registry[*worldstate.State]
	ctx := &sim.Context{Self: worldEngineID, Tick: 3}
	action.WorldZoneAddAction{Tiles: tiles}.Apply(ctx, worldstate.NewState(), &reg, nil)
	action.WorldZoneLayerAddAction{ZoneID: 7, Capacity: 20, RegenEvery: 15}.Apply(
		ctx, worldstate.NewState(), &reg, nil,
	)
	action.WorldZoneLayerAddAction{ZoneID: 7, Capacity: 20}.Apply(ctx, worldstate.NewState(), &reg,
		nil)
	requests := reg.TakeDirectiveRequests()
	if len(requests) != 3 {
		t.Fatalf("%d requests, want 3", len(requests))
	}
	zone, ok := requests[0].State.(*zonestate.State)
	if requests[0].Kind != sim.DirectiveCreateEngine || !ok || zone.WorldID() != worldEngineID ||
		requests[0].Tag[0] != worldstate.TagZone {
		t.Fatalf("the request for the zone is %+v", requests[0])
	}
	layer, ok := requests[1].State.(*zonelayerstate.State)
	if !ok || layer.ZoneID() != 7 || layer.WorldID() != worldEngineID ||
		requests[1].Tag[0] != worldstate.TagZoneLayer {
		t.Fatalf("the request for the zonelayer is %+v", requests[1])
	}
	start := requests[1].StartActions
	if len(start) != 2 || start[0] != (action.ZoneLayerAnnounceToZoneAction{Capacity: 20}) ||
		start[1] != (action.ZoneLayerCharacterRegenAction{Every: 15}) {
		t.Fatalf("the zonelayer starts with %+v, want to announce itself and heal", start)
	}
	if got := requests[2].StartActions; len(got) != 1 {
		t.Fatalf("a zonelayer that heals never starts with %+v, want only the announce", got)
	}
}

func TestAZoneLayerThatStartsTellsItsZoneWhoItIsAndTheZoneKnowsItFromThen(t *testing.T) {
	layerID := sim.NewStateID(state.ZoneLayerType, 5)
	layer := zonelayerstate.NewState(9, worldEngineID)
	var reg sim.Registry[*zonelayerstate.State]
	action.ZoneLayerAnnounceToZoneAction{Capacity: 6}.Apply(
		&sim.Context{Self: layerID}, layer, &reg, nil,
	)
	messages := reg.Take()
	if len(messages) != 1 || messages[0].Dest() != 9 {
		t.Fatalf("the messages are %v, want one to the zone 9", messages)
	}
	add := messages[0].(*sim.ActionMessage[*zonestate.State]).Actions[0]
	if add != (action.ZoneZoneLayerAddAction{ZoneLayerID: layerID, Capacity: 6}) {
		t.Fatalf("the zone is told %+v", add)
	}
	tiles, err := tilemap.New(2, 2, tilemap.Tile{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	zone := zonestate.NewState(worldEngineID, tiles)
	add.Apply(&sim.Context{}, zone, nil, nil)
	if slots := zone.ZoneLayerCount(); slots != 1 {
		t.Fatalf("the zone knows %d zonelayers, want 1", slots)
	}
}
