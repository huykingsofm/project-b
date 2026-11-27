package action_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
)

// pickupLayerID is the ID of the engine of the zonelayers of these tests.
const pickupLayerID sim.StateID = 77

// newPickupTestLayer makes a zonelayer with characters 1 and 2 standing in the middle of tiles
// (1,2) and (9,2) in a map 10 by 5, and an item lying in the middle of tile (2,2) It returns the
// layer, its registry and the entity of the item.
func newPickupTestLayer(
	t *testing.T,
) (*zonelayerstate.State, *sim.Registry[*zonelayerstate.State], state.EntityID) {
	st, reg := newMoveTestWorld(t, true)
	st.AddCharacterEntity(newCharacter(2, "b", 100, 1000, 0), state.Vec{X: 9500, Y: 2500})
	return st, reg, st.AddItemEntity(7, 1, state.Vec{X: 2500, Y: 2500}, 0)
}

func pickup(
	characterID state.CharacterID,
	item state.EntityID,
	tick uint64,
	st *zonelayerstate.State,
	reg *sim.Registry[*zonelayerstate.State],
) {
	a := action.ZoneLayerCharacterItemPickupAction{CharacterID: characterID, ItemEntityID: item}
	ctx := &sim.Context{Self: pickupLayerID, Tick: tick}
	a.Apply(ctx, st, reg, sim.NewPlan[*zonelayerstate.State]())
}

func TestAnItemWithinTheRadiusIsTakenOffTheGroundAndSentToTheWorld(t *testing.T) {
	st, reg, item := newPickupTestLayer(t)
	pickup(1, item, 10, st, reg)

	if _, ok := st.EntityByID(item); ok {
		t.Fatal("the item is still on the ground")
	}
	if got := len(reg.Take()); got != 1 {
		t.Fatalf("%d messages were sent, want 1 to the world", got)
	}
}

func TestAnItemOutsideTheRadiusStaysOnTheGround(t *testing.T) {
	st, reg, item := newPickupTestLayer(t)
	pickup(2, item, 10, st, reg) // seven tiles away; the radius is 2 tiles

	if _, ok := st.ItemEntityByID(item); !ok {
		t.Fatal("an item out of reach was taken")
	}
	if got := len(reg.Take()); got != 0 {
		t.Fatalf("%d messages were sent, want none", got)
	}
}

func TestTheReachIsASquareOfUnitsAroundTheItem(t *testing.T) {
	st, reg, _ := newPickupTestLayer(t)
	// The character is at (1500, 2500). The reach is 2000 units on every side of the item's point.
	inside := st.AddItemEntity(7, 1, state.Vec{X: 3500, Y: 4500}, 0)
	pickup(1, inside, 10, st, reg)
	if _, ok := st.ItemEntityByID(inside); ok {
		t.Fatal("an item exactly at the edge of the reach, on the diagonal, was out of reach")
	}
	outside := st.AddItemEntity(7, 1, state.Vec{X: 3501, Y: 2500}, 0)
	pickup(1, outside, 10, st, reg)
	if _, ok := st.ItemEntityByID(outside); !ok {
		t.Fatal("an item one unit past the reach was taken")
	}
}

func TestABigBodyReachesAnItemWithItsEdgeNotItsCentre(t *testing.T) {
	st, reg, _ := newPickupTestLayer(t)
	big := newCharacter(5, "boss", 1000, 1000, 1500)
	st.AddCharacterEntity(big, state.Vec{X: 5500, Y: 4500})
	// The edge of the body is at 4000 on x, and the reach, 2000 units, is from there; the centre is
	// 3500 from the item.
	near := st.AddItemEntity(7, 1, state.Vec{X: 2000, Y: 4500}, 0)
	pickup(5, near, 10, st, reg)
	if _, ok := st.ItemEntityByID(near); ok {
		t.Fatal("an item within the reach of the edge of the body was out of reach")
	}
	far := st.AddItemEntity(7, 1, state.Vec{X: 1999, Y: 4500}, 0)
	pickup(5, far, 10, st, reg)
	if _, ok := st.ItemEntityByID(far); !ok {
		t.Fatal("an item one unit past the reach of the edge of the body was taken")
	}
}

func TestAStackIsPickedUpWholeAndTheWorldGetsItsCountAndWhereItCameFrom(t *testing.T) {
	st, reg, _ := newPickupTestLayer(t)
	at := state.Vec{X: 2500, Y: 2500}
	stack := st.AddItemEntity(8, 5, at, 40)
	pickup(1, stack, 10, st, reg)
	messages := reg.Take()
	if len(messages) != 1 {
		t.Fatalf("%d messages were sent, want 1", len(messages))
	}
	if _, ok := st.ItemEntityByID(stack); ok {
		t.Fatal("the stack is still on the ground")
	}
	toWorld := messages[0].(*sim.ActionMessage[*worldstate.State])
	add, ok := toWorld.Actions[0].(action.WorldCharacterItemAddAction)
	if !ok {
		t.Fatalf("the message does not carry a WorldCharacterItemAddAction: %+v", messages[0])
	}
	want := action.WorldCharacterItemAddAction{
		CharacterID: 1, ItemID: 8, Count: 5, FromZoneLayerID: pickupLayerID, At: at, Expiry: 40,
	}
	if add != want {
		t.Fatalf("the message is %+v, want %+v", add, want)
	}
}

func TestWhenTheMessageToTheWorldIsNotDeliveredTheItemComesBackAsItWas(t *testing.T) {
	st, reg, _ := newPickupTestLayer(t)
	at := state.Vec{X: 2500, Y: 2500}
	stack := st.AddItemEntity(8, 5, at, 40)
	pickup(1, stack, 10, st, reg)
	sent := reg.Take()[0]

	back, ok := sent.OnFail().(onfail.Compensate)
	if !ok {
		t.Fatalf("a failed delivery means %T, want to compensate", sent.OnFail())
	}
	scoped := back.With.(*sim.ActionMessage[*zonelayerstate.State])
	put, ok := scoped.Actions[0].(action.ZoneLayerItemEntityAddAction)
	if !ok {
		t.Fatalf("the compensation does not carry a ZoneLayerItemEntityAddAction: %+v", back.With)
	}
	want := action.ZoneLayerItemEntityAddAction{ItemID: 8, Count: 5, At: at, Expiry: 40}
	if put != want {
		t.Fatalf("the compensation is %+v, want %+v", put, want)
	}
	put.Apply(&sim.Context{Tick: 12}, st, reg, sim.NewPlan[*zonelayerstate.State]())
	var found entityItem
	for _, id := range st.EntityIDs() {
		if item, ok := st.ItemEntityByID(id); ok {
			found = entityItem{item.ItemID(), item.Count(), item.Position()}
		}
	}
	if found != (entityItem{8, 5, at}) {
		t.Fatalf("the item on the ground is %+v, want kind 8, count 5, same place", found)
	}
}

type entityItem struct {
	itemID state.ItemID
	count  uint32
	pos    state.Vec
}

func TestTwoCharactersPickingUpTheSameItemInOneTickOnlyOneGetsIt(t *testing.T) {
	st, reg, item := newPickupTestLayer(t)
	st.AddCharacterEntity(newCharacter(3, "c", 100, 1000, 0), state.Vec{X: 1500, Y: 1500})
	pickup(1, item, 10, st, reg)
	pickup(3, item, 10, st, reg)

	if got := len(reg.Take()); got != 1 {
		t.Fatalf("%d messages were sent, want 1: the item was given twice", got)
	}
}

func TestAnItemAlreadyTakenOrNotAnItemIsNotPickedUp(t *testing.T) {
	st, reg, item := newPickupTestLayer(t)
	pickup(1, item, 10, st, reg)
	reg.Take()
	pickup(1, item, 11, st, reg)
	pickup(1, state.EntityID(1), 11, st, reg) // the entity of the character itself
	pickup(1, state.EntityID(999), 11, st, reg)
	if got := len(reg.Take()); got != 0 {
		t.Fatalf("%d messages were sent for things that cannot be picked up", got)
	}
}

func TestACharacterWithNoHealthOrNotInTheZonelayerPicksNothingUp(t *testing.T) {
	st, reg, item := newPickupTestLayer(t)
	st.DamageEntity(state.EntityID(1), 1000)
	pickup(1, item, 10, st, reg)
	pickup(42, item, 10, st, reg)
	if _, ok := st.ItemEntityByID(item); !ok {
		t.Fatal("an item was taken by a character that could not")
	}
}
