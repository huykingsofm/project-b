package action

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
)

func TestAddingAnItemSetsItsRemovalAtTheEndOfItsExpiry(t *testing.T) {
	st, reg, _ := newPickupTestLayer(t)
	const expiry = 25
	(&ZoneLayerItemEntityAddAction{
		ItemID: 8, Count: 5, At: state.Vec{X: 2500, Y: 2500}, Expiry: expiry,
	}).
		Apply(&sim.Context{Tick: 30}, st, reg, sim.NewPlan[*zonelayerstate.State]())

	_, later := reg.TakeSelf()
	if len(later) != 1 {
		t.Fatalf("%d actions were set for later, want 1", len(later))
	}
	remove, ok := later[0].Action.(*ZoneLayerItemEntityRemoveAction)
	if !ok || later[0].At != 30+expiry {
		t.Fatalf("set %+v at %d, want a removal at %d", later[0].Action, later[0].At, 30+expiry)
	}
	if _, ok := st.ItemEntityByID(remove.EntityID); !ok {
		t.Fatal("the removal is not of the item that was put on the ground")
	}
}

func TestTheRemovalTakesTheItemOffTheGround(t *testing.T) {
	st, reg, item := newPickupTestLayer(t)
	(&ZoneLayerItemEntityRemoveAction{EntityID: item}).Apply(
		&sim.Context{Tick: 99}, st, reg, sim.NewPlan[*zonelayerstate.State](),
	)
	if _, ok := st.ItemEntityByID(item); ok {
		t.Fatal("the item is still on the ground")
	}
}

func TestTheRemovalOfAnItemThatWasPickedUpDoesNothing(t *testing.T) {
	st, reg, item := newPickupTestLayer(t)
	pickup(1, item, 10, st, reg)
	// The stack comes back with a new entity; the old removal must not take it.
	(&ZoneLayerItemEntityAddAction{
		ItemID: 7, Count: 1, At: state.Vec{X: 2500, Y: 2500}, Expiry: 25,
	}).
		Apply(&sim.Context{Tick: 11}, st, reg, sim.NewPlan[*zonelayerstate.State]())
	before := st.Entities.Len()
	(&ZoneLayerItemEntityRemoveAction{EntityID: item}).Apply(
		&sim.Context{Tick: 99}, st, reg, sim.NewPlan[*zonelayerstate.State](),
	)
	if st.Entities.Len() != before {
		t.Fatal("the removal of a picked up item took another item")
	}
}

func TestAnItemWithNoExpiryIsNeverRemoved(t *testing.T) {
	st, reg, _ := newPickupTestLayer(t)
	(&ZoneLayerItemEntityAddAction{ItemID: 8, Count: 5, At: state.Vec{X: 2500, Y: 2500}}).
		Apply(&sim.Context{Tick: 30}, st, reg, sim.NewPlan[*zonelayerstate.State]())
	if _, later := reg.TakeSelf(); len(later) != 0 {
		t.Fatalf("%d removals were set, want none", len(later))
	}
}
