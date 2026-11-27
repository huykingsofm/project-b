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
	messages := reg.Take()
	if len(messages) != 3 {
		t.Fatalf("%d messages, want 3", len(messages))
	}
	asked := make([]sim.CreateEngineAction, len(messages))
	for i, m := range messages {
		if m.Dest() != sim.CoordinatorStateID {
			t.Fatalf("the message %d is for %v, want the coordinator", i, m.Dest())
		}
		asked[i] = m.(*sim.ActionMessage[*sim.CoordinatorState]).Actions[0].(sim.CreateEngineAction)
	}
	zone, ok := asked[0].State.(*zonestate.State)
	if !ok || zone.WorldID() != worldEngineID {
		t.Fatalf("the request for the zone is %+v", asked[0])
	}
	if got := asked[0].Start; len(got) != 1 || got[0] != (action.ZoneAnnounceToWorldAction{}) {
		t.Fatalf("the zone starts with %+v, want to announce itself to the world", got)
	}
	layer, ok := asked[1].State.(*zonelayerstate.State)
	if !ok || layer.ZoneID() != 7 || layer.WorldID() != worldEngineID {
		t.Fatalf("the request for the zonelayer is %+v", asked[1])
	}
	start := asked[1].Start
	if len(start) != 3 || start[0] != (action.ZoneLayerAnnounceToZoneAction{Capacity: 20}) ||
		start[1] != (action.ZoneLayerAnnounceToWorldAction{}) ||
		start[2] != (action.ZoneLayerCharacterRegenAction{Every: 15}) {
		t.Fatalf("the zonelayer starts with %+v, want to announce itself twice and heal", start)
	}
	if got := asked[2].Start; len(got) != 2 {
		t.Fatalf("a zonelayer that heals never starts with %+v, want only the announces", got)
	}
}

func TestTheWorldListsTheZoneAndTheZoneLayerThatTellItTheyAreThere(t *testing.T) {
	world := worldstate.NewState()
	zoneID := sim.NewStateID(state.ZoneType, 3)
	layerID := sim.NewStateID(state.ZoneLayerType, 8)
	action.WorldZoneRegisterAction{ZoneID: zoneID}.Apply(&sim.Context{}, world, nil, nil)
	action.WorldZoneLayerRegisterAction{ZoneLayerID: layerID}.Apply(&sim.Context{}, world, nil, nil)
	if got := world.ZoneIDs(); len(got) != 1 || got[0] != zoneID {
		t.Fatalf("the world lists the zones %v, want %v", got, zoneID)
	}
	if got := world.ZoneLayerIDs(); len(got) != 1 || got[0] != layerID {
		t.Fatalf("the world lists the zonelayers %v, want %v", got, layerID)
	}
}

func TestAZoneAndAZoneLayerThatStartTellTheirWorldWhoTheyAre(t *testing.T) {
	zoneID := sim.NewStateID(state.ZoneType, 3)
	layerID := sim.NewStateID(state.ZoneLayerType, 8)
	tiles, err := tilemap.New(2, 2, tilemap.Tile{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var zoneReg sim.Registry[*zonestate.State]
	action.ZoneAnnounceToWorldAction{}.Apply(
		&sim.Context{Self: zoneID}, zonestate.NewState(worldEngineID, tiles), &zoneReg, nil,
	)
	var layerReg sim.Registry[*zonelayerstate.State]
	action.ZoneLayerAnnounceToWorldAction{}.Apply(
		&sim.Context{Self: layerID}, zonelayerstate.NewState(9, worldEngineID), &layerReg, nil,
	)
	zoneMessages, layerMessages := zoneReg.Take(), layerReg.Take()
	if len(zoneMessages) != 1 || zoneMessages[0].Dest() != worldEngineID ||
		len(layerMessages) != 1 || layerMessages[0].Dest() != worldEngineID {
		t.Fatalf("the messages are %v and %v, want one each to the world", zoneMessages,
			layerMessages)
	}
	listed := zoneMessages[0].(*sim.ActionMessage[*worldstate.State]).Actions[0]
	if listed != (action.WorldZoneRegisterAction{ZoneID: zoneID}) {
		t.Fatalf("the world is told %+v", listed)
	}
	listed = layerMessages[0].(*sim.ActionMessage[*worldstate.State]).Actions[0]
	if listed != (action.WorldZoneLayerRegisterAction{ZoneLayerID: layerID}) {
		t.Fatalf("the world is told %+v", listed)
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
