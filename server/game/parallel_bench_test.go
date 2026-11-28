package game_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap/tilemaptest"
	"github.com/huykingsofm/project-b/server/game/testkit"
)

// BenchmarkZoneLayersInParallel is a tick of every one of n zonelayers of a zone, each holding 150
// characters, each on a goroutine of its own, as the cluster runs them: the time of one tick of
// the whole set. Run it with -cpu 1,2,4 to see what more cores give.
func BenchmarkZoneLayersInParallel(b *testing.B) {
	for _, n := range []int{1, 2, 4, 8} {
		b.Run(fmt.Sprintf("layers=%d", n), func(b *testing.B) {
			layers := make([]testkit.LayerSpec, n)
			for i := range layers {
				layers[i] = testkit.LayerSpec{Capacity: 200}
			}
			c := testkit.Make(b, testkit.Options(b, 7), nil, testkit.ZoneSpec{
				Tiles:  tilemaptest.New(b, 10, 5, tilemap.Tile{X: 1, Y: 2}, nil),
				Layers: layers,
			})
			engines := make([]*engine.Engine[*zonelayerstate.State], n)
			for i := range engines {
				id := sim.NewStateID(state.ZoneLayerType, uint64(i+1))
				engines[i] = fillWithPlayers(b, c.ZoneLayer(id))
			}
			b.ReportAllocs()
			b.ResetTimer()
			var wg sync.WaitGroup
			for _, e := range engines {
				wg.Go(func() {
					for range b.N {
						if err := e.Step(); err != nil {
							b.Error(err)
							return
						}
					}
				})
			}
			wg.Wait()
		})
	}
}
