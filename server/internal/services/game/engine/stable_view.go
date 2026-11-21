package engine

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
)

// StableStateView is what one engine reads of the StableStateRegistry in a tick. The first time the engine reads a scope in
// the tick it keeps the copy it got, and every later read of that scope in the tick returns the same
// copy, so what an engine knows of the others does not change while its plan runs (the registry itself
// gives the latest copy at the moment of the read, and the other engines publish at any time). The
// first read of each scope in a tick is written to the replay log, with the tick of the copy, so that
// a replay can give the engine the same one. Only the goroutine of its engine uses it, so it takes no
// lock.
type StableStateView struct {
	registry   *StableStateRegistry
	replayLog  ReplayLog
	me         action.Address
	tick       uint64
	world      *worldstate.State
	worldRead  bool
	zones      map[state.ZoneID]*zonestate.State
	zonelayers map[state.ZoneLayerID]*zonelayerstate.State
}

var _ action.StableView = (*StableStateView)(nil)

// begin forgets what was kept: a new tick reads again.
func (v *StableStateView) begin(tick uint64) {
	v.tick, v.world, v.worldRead = tick, nil, false
	clear(v.zones)
	clear(v.zonelayers)
}

// World returns the copy of the world that this tick read first.
func (v *StableStateView) World() *worldstate.State {
	if !v.worldRead {
		v.world, v.worldRead = v.registry.latestWorld(), true
		var copyTick uint64
		if v.world != nil {
			copyTick = v.world.Tick
		}
		v.replayLog.StableRead(v.me, v.tick, action.WorldAddress(v.registry.worldID), copyTick)
	}
	return v.world
}

// Zone returns the copy of the zone that this tick read first.
func (v *StableStateView) Zone(zoneID state.ZoneID) *zonestate.State {
	if z, ok := v.zones[zoneID]; ok {
		return z
	}
	z := v.registry.latestZone(zoneID)
	v.zones[zoneID] = z
	var copyTick uint64
	if z != nil {
		copyTick = z.Tick
	}
	v.replayLog.StableRead(v.me, v.tick, action.ZoneAddress(zoneID), copyTick)
	return z
}

// ZoneLayer returns the copy of the zonelayer that this tick read first.
func (v *StableStateView) ZoneLayer(zoneLayerID state.ZoneLayerID) *zonelayerstate.State {
	if l, ok := v.zonelayers[zoneLayerID]; ok {
		return l
	}
	l := v.registry.latestZoneLayer(zoneLayerID)
	v.zonelayers[zoneLayerID] = l
	var copyTick uint64
	if l != nil {
		copyTick = l.Tick
	}
	v.replayLog.StableRead(v.me, v.tick, action.ZoneLayerAddress(zoneLayerID), copyTick)
	return l
}
