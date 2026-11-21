package engine

import (
	"maps"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
)

type graphData struct {
	zones      []state.ZoneID
	adj        map[state.ZoneID][]state.ZoneID
	zonelayers map[state.ZoneID][]state.ZoneLayerID
	zoneOf     map[state.ZoneLayerID]state.ZoneID
}

func (g *graphData) clone() *graphData {
	n := &graphData{
		zones:      slices.Clone(g.zones),
		adj:        make(map[state.ZoneID][]state.ZoneID, len(g.adj)),
		zonelayers: make(map[state.ZoneID][]state.ZoneLayerID, len(g.zonelayers)),
		zoneOf:     make(map[state.ZoneLayerID]state.ZoneID, len(g.zoneOf)),
	}
	for zoneID, v := range g.adj {
		n.adj[zoneID] = slices.Clone(v)
	}
	for zoneID, v := range g.zonelayers {
		n.zonelayers[zoneID] = slices.Clone(v)
	}
	maps.Copy(n.zoneOf, g.zoneOf)
	return n
}

// WorldGraph is how the world is laid out: one world, its zones, which zones lead to which,
// and the zonelayers of each zone. It is read by every engine and changed when a zone or a zonelayer is
// added or removed, which is rare, so it is replaced as a whole and read without a lock.
type WorldGraph struct {
	worldID state.WorldID
	mu      sync.Mutex
	p       atomic.Pointer[graphData]
}

var _ action.Graph = (*WorldGraph)(nil)

// NewWorldGraph makes the graph of an empty world.
func NewWorldGraph(worldID state.WorldID) *WorldGraph {
	g := &WorldGraph{worldID: worldID}
	g.p.Store(&graphData{adj: map[state.ZoneID][]state.ZoneID{}, zonelayers: map[state.ZoneID][]state.ZoneLayerID{}, zoneOf: map[state.ZoneLayerID]state.ZoneID{}})
	return g
}

func (g *WorldGraph) change(f func(*graphData)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := g.p.Load().clone()
	f(n)
	g.p.Store(n)
}

// AddZone adds a zone that leads to the zones in adjacent.
func (g *WorldGraph) AddZone(zoneID state.ZoneID, adjacent ...state.ZoneID) {
	g.change(func(d *graphData) {
		if !slices.Contains(d.zones, zoneID) {
			d.zones = append(d.zones, zoneID)
		}
		d.adj[zoneID] = slices.Clone(adjacent)
	})
}

// AddZoneLayer adds a zonelayer to a zone.
func (g *WorldGraph) AddZoneLayer(zoneID state.ZoneID, zoneLayerID state.ZoneLayerID) {
	g.change(func(d *graphData) {
		d.zonelayers[zoneID] = append(d.zonelayers[zoneID], zoneLayerID)
		d.zoneOf[zoneLayerID] = zoneID
	})
}

// RemoveZoneLayer removes a zonelayer.
func (g *WorldGraph) RemoveZoneLayer(removedZoneLayerID state.ZoneLayerID) {
	g.change(func(d *graphData) {
		zoneID := d.zoneOf[removedZoneLayerID]
		d.zonelayers[zoneID] = slices.DeleteFunc(d.zonelayers[zoneID], func(zoneLayerID state.ZoneLayerID) bool { return zoneLayerID == removedZoneLayerID })
		delete(d.zoneOf, removedZoneLayerID)
	})
}

// World returns the ID of the world.
func (g *WorldGraph) World() state.WorldID { return g.worldID }

// Zones returns the zones, in the order they were added. The slice is a view: do not change it.
func (g *WorldGraph) Zones() []state.ZoneID { return g.p.Load().zones }

// AdjZones returns the zones that a zone leads to.
func (g *WorldGraph) AdjZones(zoneID state.ZoneID) []state.ZoneID { return g.p.Load().adj[zoneID] }

// ZoneLayersOfZone returns the zonelayers of a zone.
func (g *WorldGraph) ZoneLayersOfZone(zoneID state.ZoneID) []state.ZoneLayerID {
	return g.p.Load().zonelayers[zoneID]
}

// ZoneOfZoneLayer returns the zone that a zonelayer belongs to.
func (g *WorldGraph) ZoneOfZoneLayer(zoneLayerID state.ZoneLayerID) (state.ZoneID, bool) {
	zoneID, ok := g.p.Load().zoneOf[zoneLayerID]
	return zoneID, ok
}
