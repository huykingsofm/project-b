package world_test

import (
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/world"
	"github.com/huykingsofm/project-b/server/game/sim/state/world/event"
	"github.com/huykingsofm/project-b/shared/testing/aliasing"
)

func TestItemsFillTheCellsOfACharacterInOrderAndWhatDoesNotFitIsReturned(t *testing.T) {
	st := world.NewState(1)
	add := func(char state.CharacterID, item state.ItemID, count uint32) uint32 {
		return st.AddItemToCharacter(char, item, count, 3, 10)
	}
	if left := add(1, 7, 4); left != 0 {
		t.Fatalf("left = %d, want 0", left)
	}
	// 4 more of the same kind go to the slot that already holds it, up to 10; the rest to a new
	// slot.
	if left := add(1, 7, 9); left != 0 {
		t.Fatalf("left = %d, want 0", left)
	}
	slots := st.InventorySlots(1)
	want := [][2]uint32{{7, 10}, {7, 3}, {0, 0}} // the kind and the count of each slot
	if len(slots) != len(want) {
		t.Fatalf("%d slots, want %d", len(slots), len(want))
	}
	for i, slot := range slots {
		if uint32(slot.ItemID()) != want[i][0] || slot.Count() != want[i][1] {
			t.Fatalf("slot %d holds %d of %d, want %d of %d",
				i, slot.Count(), slot.ItemID(), want[i][1], want[i][0])
		}
	}
	if left := add(1, 8, 1); left != 0 {
		t.Fatalf("left = %d, want 0", left)
	}
	// The three slots are full of kinds, so another kind does not fit, and 8 more of kind 7 fit in
	// the slot that has 3 up to 10 (7 of them), with 1 left.
	if left := add(1, 9, 5); left != 5 {
		t.Fatalf("a kind with no slot left: left = %d, want 5", left)
	}
	if left := add(1, 7, 8); left != 1 {
		t.Fatalf("left = %d, want 1", left)
	}
	if got := st.ItemsHeld(1, 7); got != 20 {
		t.Fatalf("character 1 has %d of item 7, want 20", got)
	}
	if got := st.ItemsHeld(2, 7); got != 0 {
		t.Fatalf("character 2 has %d of item 7, want 0", got)
	}
	if left := add(2, 7, 1); left != 0 || st.ItemsHeld(2, 7) != 1 {
		t.Fatal("the inventory of character 2 is not its own")
	}
}

func TestOnlyWhatWasAddedIsTold(t *testing.T) {
	st := world.NewState(1)
	st.AddItemToCharacter(1, 7, 15, 1, 10) // 10 fit, 5 do not
	events := st.TakeEvents()
	added, ok := events[0].(event.CharacterItemAdded)
	if len(events) != 1 || !ok || added.Count != 10 {
		t.Fatalf("events = %+v, want one with a count of 10", events)
	}
	st.AddItemToCharacter(1, 7, 3, 1, 10) // nothing fits
	if got := st.TakeEvents(); len(got) != 0 {
		t.Fatalf("an addition that added nothing told %+v", got)
	}
}

// R29: nothing in a state is shared with its clone.
func TestAChangeAfterCloneDoesNotChangeTheCopyAndTheOtherWayRound(t *testing.T) {
	st := world.NewState(1)
	st.AddItemToCharacter(1, 7, 1, 3, 10)
	before := st.Hash()
	copied := st.Clone().(*world.State)

	st.AddItemToCharacter(1, 7, 5, 3, 10)
	if copied.Hash() != before || copied.ItemsHeld(1, 7) != 1 {
		t.Fatal("a change of the state changed its copy")
	}
	copied.AddItemToCharacter(2, 8, 1, 3, 10)
	if st.ItemsHeld(2, 8) != 0 {
		t.Fatal("a change of the copy changed the state")
	}
}

func TestAClonedWorldStateSharesNoMemoryWithTheState(t *testing.T) {
	st := world.NewState(1)
	st.AddItemToCharacter(1, 7, 1, 3, 10)
	st.SendPrivateMessage(world.NewPrivateChatMessage(1, 2, "hi", 0))
	if err := aliasing.Disjoint(st, st.Clone()); err != nil {
		t.Fatal(err)
	}
}

func TestTheTypesOfTheWorldStateObeyTheRuleOfData(t *testing.T) {
	if err := aliasing.CheckCloneable(
		reflect.TypeFor[world.State](), aliasing.Rules{Skip: []string{"world.State.events"}},
	); err != nil {
		t.Fatal(err)
	}
}
