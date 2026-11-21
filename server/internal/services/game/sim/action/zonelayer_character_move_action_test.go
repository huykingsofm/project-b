package action

import (
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap"
)

// testStable is a StableView that holds the state of one zone.
type testStable struct{ zone *zonestate.State }

func (t testStable) World() *worldstate.State                          { return nil }
func (t testStable) Zone(zoneID state.ZoneID) *zonestate.State         { return t.zone }
func (t testStable) ZoneLayer(state.ZoneLayerID) *zonelayerstate.State { return nil }

func newMoveTestWorld(zonePublished bool) (*zonelayerstate.State, *ZoneLayerRegistry) {
	st := zonelayerstate.NewState(1, 1)
	st.AttachCharacter(zonelayerstate.NewCharacter(1, "a", 100), state.Vec{X: 1, Y: 2})
	reg := &ZoneLayerRegistry{}
	if zonePublished {
		tiles := tilemap.New(10, 5, state.Vec{X: 1, Y: 2}, nil, nil)
		reg.Common = Common{Stable: testStable{zone: zonestate.NewState(1, tiles)}}
	} else {
		reg.Common = Common{Stable: testStable{}}
	}
	return st, reg
}

func TestARunTakesItsPlaceInTheCooldownAndAMoveCanBeCancelled(t *testing.T) {
	st, reg := newMoveTestWorld(true)
	plan := NewPlan[ZoneLayerAction]()
	run := &ZoneLayerCharacterMoveAction{CharID: 1, Dir: state.East, Ticks: 5}

	run.ApplyZoneLayer(&Context{Tick: 10}, st, reg, plan)
	if got := st.CooldownUntil[ZoneLayerCharacterMoveActionCooldownKey](1); got != 15 {
		t.Fatalf("the cooldown ends at %d, want 15", got)
	}
	if e, _ := st.EntityOfCharacter(1); e.Pos != (state.Vec{X: 2, Y: 2}) {
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
	st, reg := newMoveTestWorld(false)
	run := &ZoneLayerCharacterMoveAction{CharID: 1, Dir: state.East, Ticks: 3}
	run.ApplyZoneLayer(&Context{Tick: 10}, st, reg, NewPlan[ZoneLayerAction]())

	if e, _ := st.EntityOfCharacter(1); e.Pos != (state.Vec{X: 1, Y: 2}) {
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
	st, _ := newMoveTestWorld(true)
	st.SetCooldown[ZoneLayerCharacterMoveActionCooldownKey](1, 99)
	if got := st.CooldownUntil[otherCooldownKey](1); got != 0 {
		t.Fatalf("the other cooldown is %d, want none", got)
	}
}
