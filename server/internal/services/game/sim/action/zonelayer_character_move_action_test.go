package action

import (
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap/tilemaptest"
)

// testStable is a StableView that holds the state of one zone.
type testStable struct{ zone *zonestate.State }

func (t testStable) World() *worldstate.State                          { return nil }
func (t testStable) Zone(zoneID state.ZoneID) *zonestate.State         { return t.zone }
func (t testStable) ZoneLayer(state.ZoneLayerID) *zonelayerstate.State { return nil }

func newMoveTestWorld(t *testing.T, zonePublished bool) (*zonelayerstate.State, *ZoneLayerRegistry) {
	st := zonelayerstate.NewState(1, 1)
	st.AddCharacterEntity(newCharacter(1, "a", 100, 1000, 0), state.Vec{X: 1500, Y: 2500})
	reg := &ZoneLayerRegistry{}
	if zonePublished {
		tiles := tilemaptest.New(t, 10, 5, tilemap.Tile{X: 1, Y: 2}, nil, nil)
		reg.Common = Common{Stable: testStable{zone: zonestate.NewState(1, tiles)}}
	} else {
		reg.Common = Common{Stable: testStable{}}
	}
	return st, reg
}

func TestARunTakesItsPlaceInTheCooldownAndAMoveCanBeCancelled(t *testing.T) {
	st, reg := newMoveTestWorld(t, true)
	plan := NewPlan[ZoneLayerAction]()
	run := &ZoneLayerCharacterMoveAction{CharacterID: 1, Dir: state.East, Ticks: 5}

	run.ApplyZoneLayer(&Context{Tick: 10}, st, reg, plan)
	if got := st.CooldownUntil[ZoneLayerCharacterMoveActionCooldownKey](1); got != 15 {
		t.Fatalf("the cooldown ends at %d, want 15", got)
	}
	if e, _ := st.CharacterEntityByCharacterID(1); e.Pos != (state.Vec{X: 2500, Y: 2500}) {
		t.Fatalf("position = %+v, want one step east", e.Pos)
	}

	// A run that is cancelled frees the character at once.
	var cancellable ZoneLayerCancellableAction = run
	cancellable.ApplyZoneLayerCancel(&Context{Tick: 12}, st, reg)
	if got := st.CooldownUntil[ZoneLayerCharacterMoveActionCooldownKey](1); got != 12 {
		t.Fatalf("after the cancel the cooldown ends at %d, want 12", got)
	}
}

func TestAMoveThatNeedsTheMapWaitsUntilTheZoneHasPublishedIt(t *testing.T) {
	st, reg := newMoveTestWorld(t, false)
	run := &ZoneLayerCharacterMoveAction{CharacterID: 1, Dir: state.East, Ticks: 3}
	run.ApplyZoneLayer(&Context{Tick: 10}, st, reg, NewPlan[ZoneLayerAction]())

	if e, _ := st.CharacterEntityByCharacterID(1); e.Pos != (state.Vec{X: 1500, Y: 2500}) {
		t.Fatalf("the character moved without a map: %+v", e.Pos)
	}
	next, _ := reg.TakeSelf()
	if len(next) != 1 || next[0] != ZoneLayerAction(run) {
		t.Fatal("the run did not set itself for the next tick")
	}
	if run.Ticks != 3 {
		t.Fatalf("the run used %d ticks without moving", 3-run.Ticks)
	}
}

// otherCooldownKey stands for the key of some other action.
type otherCooldownKey struct{}

func TestACooldownKeyOfOneActionIsNotTheKeyOfAnother(t *testing.T) {
	st, _ := newMoveTestWorld(t, true)
	st.SetCooldown[ZoneLayerCharacterMoveActionCooldownKey](1, 99)
	if got := st.CooldownUntil[otherCooldownKey](1); got != 0 {
		t.Fatalf("the other cooldown is %d, want none", got)
	}
}

func TestACharacterMovesByItsOwnSpeedEachTick(t *testing.T) {
	for _, speed := range []int32{250, 1000, 3000} {
		st, reg := newMoveTestWorld(t, true)
		ch, _ := st.RemoveCharacterEntity(1)
		ch.Speed = speed
		st.AddCharacterEntity(ch, state.Vec{X: 1500, Y: 2500})
		start, _ := st.CharacterEntityByCharacterID(1)

		run := &ZoneLayerCharacterMoveAction{CharacterID: 1, Dir: state.East, Ticks: 2}
		run.ApplyZoneLayer(&Context{Tick: 10}, st, reg, NewPlan[ZoneLayerAction]())
		run.ApplyZoneLayer(&Context{Tick: 11}, st, reg, NewPlan[ZoneLayerAction]())

		end, _ := st.CharacterEntityByCharacterID(1)
		if got := end.Pos.X - start.Pos.X; got != 2*speed {
			t.Errorf("with a speed of %d the character went %d units in 2 ticks, want %d", speed, got, 2*speed)
		}
	}
}

func TestACharacterWithNoSpeedDoesNotMove(t *testing.T) {
	st, reg := newMoveTestWorld(t, true)
	ch, _ := st.RemoveCharacterEntity(1)
	ch.Speed = 0
	st.AddCharacterEntity(ch, state.Vec{X: 1500, Y: 2500})
	(&ZoneLayerCharacterMoveAction{CharacterID: 1, Dir: state.East, Ticks: 3}).ApplyZoneLayer(&Context{Tick: 10}, st, reg, NewPlan[ZoneLayerAction]())
	if e, _ := st.CharacterEntityByCharacterID(1); e.Pos != (state.Vec{X: 1500, Y: 2500}) {
		t.Fatalf("a character with no speed moved to %+v", e.Pos)
	}
}

// newCharacter is a character of the values of a test, which are never invalid.
func newCharacter(characterID state.CharacterID, name string, maxHealth, speed, radius int32) zonelayerstate.Character {
	c, err := zonelayerstate.NewCharacter(characterID, name, maxHealth, speed, radius)
	if err != nil {
		panic(err)
	}
	return c
}
