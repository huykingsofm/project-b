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
	"github.com/huykingsofm/project-b/server/game/sim/tilemap/tilemaptest"
)

// the IDs of the states that these benchmarks put in a world of their own
var (
	benchZoneID      = sim.NewStateID(state.ZoneType, 1)
	benchOtherZoneID = sim.NewStateID(state.ZoneType, 2)
	benchLayerID     = sim.NewStateID(state.ZoneLayerType, 1)
	benchOtherLayer  = sim.NewStateID(state.ZoneLayerType, 2)
	benchWorldID     = sim.NewStateID(state.WorldType, 1)
)

// benchStable is a reader of the states that the actions of these benchmarks read.
type benchStable map[sim.StateID]sim.State

func (s benchStable) Read(id sim.StateID) (sim.State, error) {
	if st, found := s[id]; found {
		return st, nil
	}
	return nil, sim.ErrStateNotFound
}

// benchApply measures one Apply of the action on a copy of the state, as an engine of the state
// would run it: the registry is new for each run and is emptied after it. The copy is part of what
// is measured, so the table of the results gives the cost of the copy beside it.
func benchApply[S sim.State](
	b *testing.B, name string, base S, stable benchStable, a sim.Action[S], tick uint64,
) {
	b.Run(name, func(b *testing.B) {
		plan := sim.NewPlan[S]()
		ctx := &sim.Context{Self: benchLayerID, Tick: tick}
		b.ReportAllocs()
		for b.Loop() {
			st := base.Clone().(S)
			reg := &sim.ActionRegistry[S]{Me: ctx.Self, Tick: tick, Stable: stable}
			a.Apply(ctx, st, reg, plan)
			reg.Take()
			reg.TakeSelf()
			reg.TakeDirectiveRequests()
			plan.Reset()
		}
	})
}

// benchClone measures the copy that benchApply makes, with nothing applied.
func benchClone[S sim.State](b *testing.B, name string, base S) {
	b.Run(name, func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = base.Clone().(S)
		}
	})
}

// benchTiles is the map of the zones of these benchmarks: 10 by 5 tiles with the spawn at (1, 2).
func benchTiles(b *testing.B) *tilemap.TileMap {
	return tilemaptest.New(b, 10, 5, tilemap.Tile{X: 1, Y: 2}, nil)
}

// benchLayer is a zonelayer of the zone 1 with the characters 1 and 2 on the map and an item on
// the ground near the first.
func benchLayer(b *testing.B) (*zonelayerstate.State, state.EntityID, state.EntityID) {
	st := zonelayerstate.NewState(benchZoneID, benchWorldID)
	st.AddCharacterEntity(newCharacter(1, "a", 100, 1000, 256), state.Vec{X: 1500, Y: 2500})
	st.AddCharacterEntity(newCharacter(2, "b", 100, 1000, 256), state.Vec{X: 2000, Y: 2500})
	item := st.AddItemEntity(7, 1, state.Vec{X: 1600, Y: 2500}, 0)
	body, _ := st.CharacterEntityByCharacterID(2)
	return st, item, body.ID()
}

func BenchmarkApplyZoneLayerActions(b *testing.B) {
	layer, item, victim := benchLayer(b)
	zone := zonestate.NewState(benchWorldID, benchTiles(b))
	exitArea := state.Rect{Min: state.Vec{X: 1000, Y: 2000}, Max: state.Vec{X: 2000, Y: 3000}}
	zone.AddExit(exitArea, benchOtherZoneID, state.Vec{X: 5500, Y: 2500})
	other := zonestate.NewState(benchWorldID, benchTiles(b))
	other.AddZoneLayer(benchOtherLayer, 10)
	stable := benchStable{benchZoneID: zone, benchOtherZoneID: other}
	exitEntity := state.EntityID(1)

	benchClone(b, "the copy of the state", layer)
	benchApply(b, "Damage", layer, stable,
		sim.Action[*zonelayerstate.State](
			action.ZoneLayerCharacterDamageAction{VictimID: victim, Amount: 1},
		), 10)
	benchApply(b, "Enter", layer, stable,
		sim.Action[*zonelayerstate.State](action.ZoneLayerCharacterEnterAction{
			Character: newCharacter(3, "c", 100, 1000, 256), At: state.Vec{X: 3500, Y: 2500},
		}), 10)
	benchApply(b, "ExitTo", layer, stable,
		sim.Action[*zonelayerstate.State](
			action.ZoneLayerCharacterExitToAction{CharacterID: 1, ExitEntityID: exitEntity},
		), 10)
	benchApply(b, "ItemPickup", layer, stable,
		sim.Action[*zonelayerstate.State](
			action.ZoneLayerCharacterItemPickupAction{CharacterID: 1, ItemEntityID: item},
		), 10)
	benchApply(b, "Move", layer, stable,
		sim.Action[*zonelayerstate.State](
			action.ZoneLayerCharacterMoveAction{CharacterID: 1, Dir: state.East, Ticks: 1},
		), 10)
	benchApply(b, "NormalAttackTrigger", layer, stable,
		sim.Action[*zonelayerstate.State](
			action.ZoneLayerCharacterNormalAttackTriggerAction{CharacterID: 1, TargetID: 2},
		), 10)
	benchApply(b, "Regen", layer, stable,
		sim.Action[*zonelayerstate.State](action.ZoneLayerCharacterRegenAction{Every: 10}), 10)
	benchApply(b, "ShieldGrant", layer, stable,
		sim.Action[*zonelayerstate.State](
			action.ZoneLayerCharacterShieldGrantAction{CharacterID: 1, Duration: 5},
		), 10)
	benchApply(b, "AnnounceToZone", layer, stable,
		sim.Action[*zonelayerstate.State](action.ZoneLayerAnnounceToZoneAction{Capacity: 10}), 10)
	benchApply(b, "ChatSend", layer, stable,
		sim.Action[*zonelayerstate.State](action.ZoneLayerChatSendAction{FromID: 1, Text: "hello"}),
		10)
	benchApply(b, "ItemEntityAdd", layer, stable,
		sim.Action[*zonelayerstate.State](action.ZoneLayerItemEntityAddAction{
			ItemID: 7, Count: 2, At: state.Vec{X: 2500, Y: 2500}, Expiry: 100,
		}), 10)
	benchApply(b, "ItemEntityRemove", layer, stable,
		sim.Action[*zonelayerstate.State](action.ZoneLayerItemEntityRemoveAction{EntityID: item}),
		10)
}

func BenchmarkApplyZoneActions(b *testing.B) {
	zone := zonestate.NewState(benchWorldID, benchTiles(b))
	zone.AddZoneLayer(benchLayerID, 10)
	stable := benchStable{benchLayerID: zonelayerstate.NewState(benchZoneID, benchWorldID)}
	exitArea := state.Rect{Min: state.Vec{X: 1000, Y: 2000}, Max: state.Vec{X: 2000, Y: 3000}}

	benchClone(b, "the copy of the state", zone)
	benchApply(b, "CharacterEnter", zone, stable,
		sim.Action[*zonestate.State](action.ZoneCharacterEnterAction{
			Character: newCharacter(3, "c", 100, 1000, 256),
		}), 10)
	benchApply(b, "ChatSend", zone, stable,
		sim.Action[*zonestate.State](action.ZoneChatSendAction{FromID: 1, Text: "hello"}), 10)
	benchApply(b, "ExitAdd", zone, stable,
		sim.Action[*zonestate.State](action.ZoneExitAddAction{
			Area: exitArea, ToZoneID: benchOtherZoneID, Arrival: state.Vec{X: 5500, Y: 2500},
		}), 10)
	benchApply(b, "ZoneLayerAdd", zone, stable,
		sim.Action[*zonestate.State](
			action.ZoneZoneLayerAddAction{ZoneLayerID: benchOtherLayer, Capacity: 10},
		), 10)
}

func BenchmarkApplyWorldActions(b *testing.B) {
	world := worldstate.NewState()
	world.AddZone(benchZoneID)
	world.AddZone(benchOtherZoneID)
	exitArea := state.Rect{Min: state.Vec{X: 1000, Y: 2000}, Max: state.Vec{X: 2000, Y: 3000}}

	benchClone(b, "the copy of the state", world)
	benchApply(b, "CharacterItemAdd", world, nil,
		sim.Action[*worldstate.State](action.WorldCharacterItemAddAction{
			CharacterID: 1, ItemID: 7, Count: 2, FromZoneLayerID: benchLayerID,
			At: state.Vec{X: 2500, Y: 2500},
		}), 10)
	benchApply(b, "ChatPrivateSend", world, nil,
		sim.Action[*worldstate.State](
			action.WorldChatPrivateSendAction{FromID: 10, ToID: 20, Text: "psst"},
		), 10)
	benchApply(b, "ZoneAdd", world, nil,
		sim.Action[*worldstate.State](action.WorldZoneAddAction{Tiles: benchTiles(b)}), 10)
	benchApply(b, "ZoneExitAdd", world, nil,
		sim.Action[*worldstate.State](action.WorldZoneExitAddAction{
			FromZoneID: benchZoneID, Area: exitArea, ToZoneID: benchOtherZoneID,
			Arrival: state.Vec{X: 5500, Y: 2500},
		}), 10)
	benchApply(b, "ZoneLayerAdd", world, nil,
		sim.Action[*worldstate.State](
			action.WorldZoneLayerAddAction{ZoneID: benchZoneID, Capacity: 10, RegenEvery: 10},
		), 10)
}
