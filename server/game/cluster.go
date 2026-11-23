package game

import (
	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/sim"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
)

// Cluster is a world with all its zones and zonelayers, built and wired. It is a platform cluster
// that knows which of its engines is the world, which are zones and which are zonelayers.
type Cluster struct {
	*engine.Cluster
	World *engine.Engine[*worldstate.State]
}

// Zone returns the engine of a zone, or nil.
func (c *Cluster) Zone(zoneID sim.StateID) *engine.Engine[*zonestate.State] {
	e, _ := c.Get[*zonestate.State](zoneID)
	return e
}

// ZoneLayer returns the engine of a zonelayer, or nil.
func (c *Cluster) ZoneLayer(zoneLayerID sim.StateID) *engine.Engine[*zonelayerstate.State] {
	e, _ := c.Get[*zonelayerstate.State](zoneLayerID)
	return e
}

// Zones returns the engines of the zones, in the order they were added.
func (c *Cluster) Zones() []*engine.Engine[*zonestate.State] {
	return c.All[*zonestate.State]()
}

// ZoneLayers returns the engines of the zonelayers, in the order they were made.
func (c *Cluster) ZoneLayers() []*engine.Engine[*zonelayerstate.State] {
	return c.All[*zonelayerstate.State]()
}
