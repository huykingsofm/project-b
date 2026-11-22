package action

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world"
)

func TestAFullInventorySendsWhatDoesNotFitBackToTheGroundAsItWas(t *testing.T) {
	s := worldstate.NewState(1)
	reg := &WorldRegistry{}
	at := state.Vec{X: 2500, Y: 2500}
	// The defaults are 20 slots of 99, so 20 slots of 99 fill it.
	fill := &WorldCharacterItemAddAction{
		CharacterID: 1, ItemID: 7, Count: 20 * 99, FromZoneLayerID: 5, At: at, Expiry: 40,
	}
	fill.ApplyWorld(&Context{Tick: 10}, s, reg, NewPlan[WorldAction]())
	if got := len(reg.Take()); got != 0 {
		t.Fatalf("%d messages were sent while the inventory had room, want none", got)
	}

	more := &WorldCharacterItemAddAction{
		CharacterID: 1, ItemID: 7, Count: 5, FromZoneLayerID: 5, At: at, Expiry: 40,
	}
	more.ApplyWorld(&Context{Tick: 11}, s, reg, NewPlan[WorldAction]())
	if got := itemsHeld(s, 1, 7); got != 20*99 {
		t.Fatalf("the character has %d of the item, want a full inventory of %d", got, 20*99)
	}
	messages := reg.Take()
	if len(messages) != 1 {
		t.Fatalf("%d messages were sent, want 1 back to the zonelayer", len(messages))
	}
	back := messages[0].(*ScopedMessage[ZoneLayerAction])
	if back.To != message.ZoneLayerAddress(5) {
		t.Fatalf("the message goes to %+v, want zonelayer 5", back.To)
	}
	put := back.Actions[0].(*ZoneLayerItemEntityAddAction)
	want := ZoneLayerItemEntityAddAction{ItemID: 7, Count: 5, At: at, Expiry: 40}
	if *put != want {
		t.Fatalf("what goes back is %+v, want %+v", *put, want)
	}
}

// itemsHeld counts the items of a kind in the slots of a character; it is for the assertions of the
// tests.
func itemsHeld(st *worldstate.State, characterID state.CharacterID, itemID state.ItemID) uint32 {
	inventory, _ := st.Inventories.Get(characterID)
	var total uint32
	for _, inventorySlot := range inventory.Slots {
		if inventorySlot.Count > 0 && inventorySlot.ItemID == itemID {
			total += inventorySlot.Count
		}
	}
	return total
}
