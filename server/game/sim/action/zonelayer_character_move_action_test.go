package action_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap/tilemaptest"
)

// testStable is a StableReader that holds the state of one zone, with the ID 1.
type testStable struct{ zone *zonestate.State }

func (t testStable) Read(id sim.StateID) (sim.State, error) {
	if id != 1 || t.zone == nil {
		return nil, sim.ErrStateNotFound
	}
	return t.zone, nil
}

func newMoveTestWorld(
	t *testing.T, zonePublished bool,
) (*zonelayerstate.State, *sim.Registry[*zonelayerstate.State]) {
	st := zonelayerstate.NewState(1, 9)
	st.AddCharacterEntity(newCharacter(1, "a", 100, 1000, 0), state.Vec{X: 1500, Y: 2500})
	reg := &sim.Registry[*zonelayerstate.State]{}
	if zonePublished {
		tiles := tilemaptest.New(t, 10, 5, tilemap.Tile{X: 1, Y: 2}, nil)
		reg.Stable = testStable{zone: zonestate.NewState(9, tiles)}
	} else {
		reg.Stable = testStable{}
	}
	return st, reg
}

func TestARunTakesItsPlaceInTheCooldownAndAMoveCanBeCancelled(t *testing.T) {
	st, reg := newMoveTestWorld(t, true)
	plan := sim.NewPlan[*zonelayerstate.State]()
	run := action.ZoneLayerCharacterMoveAction{CharacterID: 1, Dir: state.East, Ticks: 5}

	run.Apply(&sim.Context{Tick: 10}, st, reg, plan)
	if got := st.CooldownUntil(1, action.ZoneLayerCharacterMoveActionCooldownKey); got != 15 {
		t.Fatalf("the cooldown ends at %d, want 15", got)
	}
	if e, _ := st.CharacterEntityByCharacterID(1); e.Position() != (state.Vec{X: 2500, Y: 2500}) {
		t.Fatalf("position = %+v, want one step east", e.Position())
	}

	// The run set itself for the next tick, as a copy that holds its place in the cooldown; that
	// copy, cancelled, frees the character at once.
	self := reg.TakeSelf()
	if len(self) != 1 {
		t.Fatalf("the run set itself %d times for a later tick, want once", len(self))
	}
	next := self[0].Action.(action.ZoneLayerCharacterMoveAction)
	var cancellable sim.Cancellable[*zonelayerstate.State] = next
	cancellable.ApplyCancel(&sim.Context{Tick: 12}, st, reg)
	if got := st.CooldownUntil(1, action.ZoneLayerCharacterMoveActionCooldownKey); got != 12 {
		t.Fatalf("after the cancel the cooldown ends at %d, want 12", got)
	}
}

func TestAMoveThatNeedsTheMapWaitsUntilTheZoneHasPublishedIt(t *testing.T) {
	st, reg := newMoveTestWorld(t, false)
	run := action.ZoneLayerCharacterMoveAction{CharacterID: 1, Dir: state.East, Ticks: 3}
	run.Apply(&sim.Context{Tick: 10}, st, reg, sim.NewPlan[*zonelayerstate.State]())

	if e, _ := st.CharacterEntityByCharacterID(1); e.Position() != (state.Vec{X: 1500, Y: 2500}) {
		t.Fatalf("the character moved without a map: %+v", e.Position())
	}
	self := reg.TakeSelf()
	if len(self) != 1 {
		t.Fatal("the run did not set itself for the next tick")
	}
	next := self[0].Action.(action.ZoneLayerCharacterMoveAction)
	if next.CharacterID != 1 || next.Ticks != 3 {
		t.Fatalf("the run set itself again as %+v, want the same run with its 3 ticks", next)
	}
}

// otherCooldownKey stands for the key of some other action.
const otherCooldownKey zonelayerstate.CooldownKey = 60000

func TestACooldownKeyOfOneActionIsNotTheKeyOfAnother(t *testing.T) {
	st, _ := newMoveTestWorld(t, true)
	st.SetCooldown(1, action.ZoneLayerCharacterMoveActionCooldownKey, 99)
	if got := st.CooldownUntil(1, otherCooldownKey); got != 0 {
		t.Fatalf("the other cooldown is %d, want none", got)
	}
}

func TestACharacterMovesByItsOwnSpeedEachTick(t *testing.T) {
	for _, speed := range []int32{250, 1000, 3000} {
		st, reg := newMoveTestWorld(t, true)
		ch, _ := st.RemoveCharacterEntity(1)
		ch.SetSpeed(speed)
		st.AddCharacterEntity(ch, state.Vec{X: 1500, Y: 2500})
		start, _ := st.CharacterEntityByCharacterID(1)

		run := action.ZoneLayerCharacterMoveAction{CharacterID: 1, Dir: state.East, Ticks: 2}
		run.Apply(&sim.Context{Tick: 10}, st, reg, sim.NewPlan[*zonelayerstate.State]())
		// what runs at the next tick is the copy that the run set for it
		self := reg.TakeSelf()
		if len(self) != 1 {
			t.Fatalf("the run set itself %d times for the next tick, want once", len(self))
		}
		self[0].Action.Apply(&sim.Context{Tick: 11}, st, reg, sim.NewPlan[*zonelayerstate.State]())

		end, _ := st.CharacterEntityByCharacterID(1)
		if got := end.Position().X - start.Position().X; got != 2*speed {
			t.Errorf(
				"with a speed of %d the character went %d units in 2 ticks, want %d",
				speed,
				got,
				2*speed,
			)
		}
	}
}

func TestACharacterWithNoSpeedDoesNotMove(t *testing.T) {
	st, reg := newMoveTestWorld(t, true)
	ch, _ := st.RemoveCharacterEntity(1)
	ch.SetSpeed(0)
	st.AddCharacterEntity(ch, state.Vec{X: 1500, Y: 2500})
	(action.ZoneLayerCharacterMoveAction{CharacterID: 1, Dir: state.East, Ticks: 3}).Apply(
		&sim.Context{Tick: 10}, st, reg, sim.NewPlan[*zonelayerstate.State](),
	)
	if e, _ := st.CharacterEntityByCharacterID(1); e.Position() != (state.Vec{X: 1500, Y: 2500}) {
		t.Fatalf("a character with no speed moved to %+v", e.Position())
	}
}

// newCharacter is a character of the values of a test, which are never invalid.
func newCharacter(
	characterID state.CharacterID, name string, maxHealth, speed, radius int32,
) zonelayerstate.Character {
	c, err := zonelayerstate.NewCharacter(characterID, name, maxHealth, speed, radius)
	if err != nil {
		panic(err)
	}
	return c
}
