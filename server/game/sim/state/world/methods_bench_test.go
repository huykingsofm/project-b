package world_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/world"
)

// BenchmarkMethods is each method of the state that an action calls, on a world of 2,000
// characters with an inventory of 20 slots each, 3 zones and 6 zonelayers. A method that makes
// something is measured with the one that takes it away, in the same run, so that the state is
// the same at each.
func BenchmarkMethods(b *testing.B) {
	st := world.NewState()
	for i := range 3 {
		st.AddZone(sim.NewStateID(state.ZoneType, uint64(i+1)))
	}
	for i := range 6 {
		st.AddZoneLayer(sim.NewStateID(state.ZoneLayerType, uint64(i+1)))
	}
	for c := range 2000 {
		for item := range 20 {
			st.AddItemToCharacter(state.CharacterID(c+1), state.ItemID(item+1), 3, 20, 99)
		}
	}
	layer := sim.NewStateID(state.ZoneLayerType, 9)
	text := world.NewPrivateChatMessage(10, 20, "psst", 10)

	b.Run("ItemsHeld", func(b *testing.B) {
		for b.Loop() {
			st.ItemsHeld(1000, 7)
		}
	})
	b.Run("InventorySlots", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			st.InventorySlots(1000)
		}
	})
	b.Run("AddItemToCharacter", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			st.AddItemToCharacter(1000, 7, 1, 20, 1<<30)
			st.TakeEvents()
		}
	})
	b.Run("AddZoneLayer and RemoveZoneLayer", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			st.AddZoneLayer(layer)
			st.RemoveZoneLayer(layer)
			st.TakeEvents()
		}
	})
	b.Run("SendPrivateMessage", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			st.SendPrivateMessage(text)
			st.TakeEvents()
		}
	})
}
