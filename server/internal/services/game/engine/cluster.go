package engine

import (
	"context"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"sync"
	"time"

	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
)

// Env is what every engine of a world shares: the router that carries their messages, the
// configuration they read, the replay log they write, the registry of the states they publish, the
// graph of the world, and the emitters where they give what changed.
type Env struct {
	Router    *Router
	Cfg       *config.Store
	ReplayLog replay.Log
	Stable    *StableStateRegistry
	Graph     *WorldGraph
	QueueSize int

	WorldEmitter     WorldEmitter
	ZoneEmitter      ZoneEmitter
	ZoneLayerEmitter ZoneLayerEmitter
}

// Cluster is a world with all its zones and zonelayers, built and wired.
type Cluster struct {
	Env        *Env
	World      *WorldEngine
	zones      []*ZoneEngine
	zonelayers []*ZoneLayerEngine
}

// ZoneLayer returns the engine of a zonelayer.
func (c *Cluster) ZoneLayer(zoneLayerID state.ZoneLayerID) *ZoneLayerEngine {
	for _, l := range c.zonelayers {
		if l.st.ID == zoneLayerID {
			return l
		}
	}
	return nil
}

// Zone returns the engine of a zone.
func (c *Cluster) Zone(zoneID state.ZoneID) *ZoneEngine {
	for _, z := range c.zones {
		if z.st.ID == zoneID {
			return z
		}
	}
	return nil
}

// Step runs one tick of every engine, one after the other: the zonelayers, then the zones, then the
// world. A message that an engine sends is in the In channel of the receiver at once, so a zone reads
// in this step what the zonelayers sent in it, and a zonelayer reads in the next step what a zone sent. It
// is the way to run the engines with a fixed order, as a test or an example needs; Run lets each
// engine go at its own pace.
func (c *Cluster) Step() {
	for _, l := range c.zonelayers {
		l.Step()
	}
	for _, z := range c.zones {
		z.Step()
	}
	c.World.Step()
}

// Run starts every engine on a goroutine of its own, ticking every period, and returns when the
// context ends and all of them have stopped. A zone or a zonelayer is not waited for by the others.
func (c *Cluster) Run(ctx context.Context, period time.Duration) {
	var wg sync.WaitGroup
	start := func(run func(context.Context, <-chan time.Time)) {
		wg.Go(func() {
			t := time.NewTicker(period)
			defer t.Stop()
			run(ctx, t.C)
		})
	}
	start(c.World.Run)
	for _, z := range c.zones {
		start(z.Run)
	}
	for _, l := range c.zonelayers {
		start(l.Run)
	}
	<-ctx.Done()
	c.Env.Router.Stop()
	wg.Wait()
}
