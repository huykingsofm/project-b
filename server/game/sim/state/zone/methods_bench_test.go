package zone_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/zone"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap/tilemaptest"
)

// BenchmarkMethods is each method of the state that an action calls, on a zone of 200 by 200 tiles
// with 10 exits and 4 zonelayers. A method that makes something is measured with the one that
// takes it away, in the same run, so that the state is the same at each.
func BenchmarkMethods(b *testing.B) {
	tiles := tilemaptest.New(b, 200, 200, tilemap.Tile{}, nil)
	st := zone.NewState(sim.NewStateID(state.WorldType, 1), tiles)
	other := sim.NewStateID(state.ZoneType, 2)
	var exit state.EntityID
	for i := range 10 {
		left := int32(1000 * (i + 1))
		area := state.Rect{
			Min: state.Vec{X: left, Y: 1000}, Max: state.Vec{X: left + 500, Y: 1500},
		}
		exit, _ = st.AddExit(area, other, state.Vec{X: 500, Y: 500})
	}
	layer := sim.NewStateID(state.ZoneLayerType, 9)
	for i := range 4 {
		st.AddZoneLayer(sim.NewStateID(state.ZoneLayerType, uint64(i+1)), 20)
	}
	text := zone.NewChatMessage(1, "hello", 10)

	b.Run("ExitEntityByID", func(b *testing.B) {
		for b.Loop() {
			st.ExitEntityByID(exit)
		}
	})
	b.Run("ZoneLayerSlots", func(b *testing.B) {
		for b.Loop() {
			for range st.ZoneLayerSlots() {
			}
		}
	})
	b.Run("AddExit", func(b *testing.B) {
		area := state.Rect{Min: state.Vec{X: 500, Y: 500}, Max: state.Vec{X: 900, Y: 900}}
		b.ReportAllocs()
		fresh := st // a zone that holds the exits of the run is not the one that is measured: it is
		// made again from the first every 1,000 exits
		for i := 0; b.Loop(); i++ {
			if i%1000 == 0 {
				fresh = st.Clone().(*zone.State)
			}
			fresh.AddExit(area, other, state.Vec{X: 500, Y: 500})
			fresh.TakeEvents()
		}
	})
	b.Run("AddZoneLayer and RemoveZoneLayer", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			st.AddZoneLayer(layer, 20)
			st.RemoveZoneLayer(layer)
			st.TakeEvents()
		}
	})
	b.Run("SendMessage", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			st.SendMessage(text)
			st.TakeEvents()
		}
	})
}
