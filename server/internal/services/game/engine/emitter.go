package engine

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	worldevent "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world/event"
	zoneevent "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone/event"
	zonelayerevent "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer/event"
)

// An emitter is where an engine gives what happened in a tick, the events, for them to reach the
// clients. What happens to it (which client hears which change, how it is encoded and sent) is
// outside the core: this is the one place where the core hands something outward, and it is an
// interface that the caller of the core fills in. Each scope has its own events, so each has its
// own emitter, and an engine is given only the one of its scope.

// WorldEmitter is the emitter of the world engine.
type WorldEmitter interface {
	EmitWorld(from message.Address, tick uint64, events []worldevent.Event)
}

// ZoneEmitter is the emitter of the zone engines.
type ZoneEmitter interface {
	EmitZone(from message.Address, tick uint64, events []zoneevent.Event)
}

// ZoneLayerEmitter is the emitter of the zonelayer engines.
type ZoneLayerEmitter interface {
	EmitZoneLayer(from message.Address, tick uint64, events []zonelayerevent.Event)
}

// Emitter is the three emitters in one value, for a caller that serves all the scopes with it.
type Emitter interface {
	WorldEmitter
	ZoneEmitter
	ZoneLayerEmitter
}

// NopEmitter discards the changes. It is the emitter of every scope until the builder is given
// another.
type NopEmitter struct{}

func (NopEmitter) EmitWorld(message.Address, uint64, []worldevent.Event)         {}
func (NopEmitter) EmitZone(message.Address, uint64, []zoneevent.Event)           {}
func (NopEmitter) EmitZoneLayer(message.Address, uint64, []zonelayerevent.Event) {}
