package engine

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
)

// StableStateView is what one engine reads of the StableStateRegistry in a tick. The first time the
// engine reads a scope in the tick it keeps the copy it got, and every later read of that scope in
// the tick returns the same copy, so what an engine knows of the others does not change while its
// plan runs (the registry itself gives the latest copy at the moment of the read, and the other
// engines publish at any time; a replay gives the copy that the log says was read). The first read
// of each scope in a tick is written to the replay log, with the tick of the copy, so that a replay
// can give the engine the same one. Only the goroutine of its engine uses it, so it takes no lock.
type StableStateView struct {
	source     CopySource
	worldID    state.WorldID
	replayLog  replay.Log
	me         message.Address
	tick       uint64
	world      *worldstate.State
	worldRead  bool
	zones      map[state.ZoneID]*zonestate.State
	zonelayers map[state.ZoneLayerID]*zonelayerstate.State
}

var _ action.StableView = (*StableStateView)(nil)

// NewStableStateView makes the view of the engine at the address, which finds the copies that the
// source gives, in the world with the ID, and writes each first read of a tick to the replay log.
func NewStableStateView(
	source CopySource, worldID state.WorldID, reader message.Address, replayLog replay.Log,
) *StableStateView {
	return &StableStateView{
		source: source, worldID: worldID, replayLog: replayLog, me: reader,
		zones:      map[state.ZoneID]*zonestate.State{},
		zonelayers: map[state.ZoneLayerID]*zonelayerstate.State{},
	}
}

// begin forgets what was kept: a new tick reads again.
func (v *StableStateView) begin(tick uint64) {
	v.tick, v.world, v.worldRead = tick, nil, false
	clear(v.zones)
	clear(v.zonelayers)
}

// World returns the copy of the world that this tick read first.
func (v *StableStateView) World() worldstate.ReadOnlyWorldState {
	if !v.worldRead {
		v.world, v.worldRead = v.source.CopyOfWorld(v.me, v.tick), true
		var copyTick uint64
		if v.world != nil {
			copyTick = v.world.Tick
		}
		v.replayLog.Append(v.me, v.tick, replay.StableRead{
			Source: message.WorldAddress(v.worldID), CopyTick: copyTick,
		})
	}
	if v.world == nil {
		return nil // not a nil pointer inside an interface
	}
	return v.world
}

// Zone returns the copy of the zone that this tick read first.
func (v *StableStateView) Zone(zoneID state.ZoneID) zonestate.ReadOnlyZoneState {
	z := v.zone(zoneID)
	if z == nil {
		return nil // not a nil pointer inside an interface
	}
	return z
}

func (v *StableStateView) zone(zoneID state.ZoneID) *zonestate.State {
	if z, ok := v.zones[zoneID]; ok {
		return z
	}
	z := v.source.CopyOfZone(v.me, v.tick, zoneID)
	v.zones[zoneID] = z
	var copyTick uint64
	if z != nil {
		copyTick = z.Tick
	}
	v.replayLog.Append(
		v.me, v.tick, replay.StableRead{Source: message.ZoneAddress(zoneID), CopyTick: copyTick},
	)
	return z
}

// ZoneLayer returns the copy of the zonelayer that this tick read first.
func (v *StableStateView) ZoneLayer(
	zoneLayerID state.ZoneLayerID,
) zonelayerstate.ReadOnlyZoneLayerState {
	l := v.zoneLayer(zoneLayerID)
	if l == nil {
		return nil // not a nil pointer inside an interface
	}
	return l
}

func (v *StableStateView) zoneLayer(zoneLayerID state.ZoneLayerID) *zonelayerstate.State {
	if l, ok := v.zonelayers[zoneLayerID]; ok {
		return l
	}
	l := v.source.CopyOfZoneLayer(v.me, v.tick, zoneLayerID)
	v.zonelayers[zoneLayerID] = l
	var copyTick uint64
	if l != nil {
		copyTick = l.Tick
	}
	v.replayLog.Append(v.me, v.tick, replay.StableRead{
		Source: message.ZoneLayerAddress(zoneLayerID), CopyTick: copyTick,
	})
	return l
}
