package action

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
)

// Graph is what an engine needs to know of how the world is laid out: which zones there are, which
// of them are next to each other, and which zonelayers a zone has. The engines give it their own
// implementation (a caller defines the interface it needs).
type Graph interface {
	World() state.WorldID
	Zones() []state.ZoneID
	AdjZones(zoneID state.ZoneID) []state.ZoneID
	ZoneLayersOfZone(zoneID state.ZoneID) []state.ZoneLayerID
	ZoneOfZoneLayer(zoneLayerID state.ZoneLayerID) (state.ZoneID, bool)
}

// StableView is the latest state that each engine published. A state that is read from it is a copy
// that its engine will never change again, so it may be read without a lock and kept; it may be a
// few ticks old, and it is nil for an engine that published none. An action that reads it acts on
// what is known, and the owner of the data decides exactly what it is.
type StableView interface {
	World() worldstate.ReadOnlyWorldState
	Zone(zoneID state.ZoneID) zonestate.ReadOnlyZoneState
	ZoneLayer(zoneLayerID state.ZoneLayerID) zonelayerstate.ReadOnlyZoneLayerState
}

// Outbox is what an engine wants to send in a tick. Nothing is sent while the actions run: the
// engine sends the messages, in the order they were put here, when the plan is done.
type Outbox struct{ messages []message.Message }

// Add puts a message in the outbox.
func (o *Outbox) Add(l message.Message) { o.messages = append(o.messages, l) }

// Take returns the messages and empties the outbox.
func (o *Outbox) Take() []message.Message {
	out := o.messages
	o.messages = nil
	return out
}

// Common is what the registry of every scope has: the outbox, and the means to read the layout of
// the world and the latest state of the other engines.
type Common struct {
	Outbox
	Me     message.Address
	Tick   uint64
	Graph  Graph
	Stable StableView
}

// Begin sets the tick that the registry is used at.
func (c *Common) Begin(tick uint64) { c.Tick = tick }

// ToZoneLayer puts a message for a zonelayer in the outbox.
func (c *Common) ToZoneLayer(
	zoneLayerID state.ZoneLayerID, onFail message.OnFail, actions ...ZoneLayerAction,
) {
	c.Add(MessageToZoneLayer(zoneLayerID, onFail, actions...))
}

// ToZone puts a message for a zone in the outbox.
func (c *Common) ToZone(zoneID state.ZoneID, onFail message.OnFail, actions ...ZoneAction) {
	c.Add(MessageToZone(zoneID, onFail, actions...))
}

// ToWorld puts a message for the world in the outbox.
func (c *Common) ToWorld(onFail message.OnFail, actions ...WorldAction) {
	c.Add(MessageToWorld(c.Graph.World(), onFail, actions...))
}

// Timed is an action with the tick it is set for.
type Timed[A Action] struct {
	At     uint64
	Action A
}

// self is what an engine sends to itself. It is a list of its own: nothing goes through a channel,
// and the engine reads it when the next tick begins.
type self[A Action] struct {
	next    []A
	later   []Timed[A]
	cancels []Cancel
}

func (b *self[A]) toSelf(a A) { b.next = append(b.next, a) }

func (b *self[A]) toSelfAt(now, tick uint64, a A) {
	if tick <= now {
		b.next = append(b.next, a)
		return
	}
	b.later = append(b.later, Timed[A]{At: tick, Action: a})
}

func (b *self[A]) take() ([]A, []Timed[A]) {
	next, later := b.next, b.later
	b.next, b.later = nil, nil
	return next, later
}

// cancelWaiting records a cancel of what the key names, with how much was set so far in this tick:
// what is set after it is not cancelled.
func (b *self[A]) cancelWaiting(key ActionKey) {
	b.cancels = append(b.cancels, Cancel{Key: key, Next: len(b.next), Later: len(b.later)})
}

func (b *self[A]) takeCancels() []Cancel {
	cancels := b.cancels
	b.cancels = nil
	return cancels
}

// ZoneLayerRegistry is what an action of a zonelayer is given besides the state: what it wants done
// outside the state of its zonelayer.
type ZoneLayerRegistry struct {
	Common
	self[ZoneLayerAction]
}

// ToSelf sets an action to run in the next tick of the zonelayer.
func (r *ZoneLayerRegistry) ToSelf(a ZoneLayerAction) { r.toSelf(a) }

// ToSelfAt sets an action to run at a tick of the zonelayer. An action that knows when it may run,
// a wait for a cooldown for example, is set for exactly that tick and not tried again at every tick
// in between.
func (r *ZoneLayerRegistry) ToSelfAt(tick uint64, a ZoneLayerAction) { r.toSelfAt(r.Tick, tick, a) }

// CancelWaiting ends sooner what waits in the zonelayer: the actions the key names that wait for
// a later tick, or for the next one, and what this tick set before the call. It takes effect when
// the tick is over. It does not reach an action that is in the plan of this tick (Plan.Cancel
// does), nor what such an action sets again after the call, so it is not what stops an action
// that sets itself again: that action decides for itself, from the state, whether it still does
// anything and whether it sets itself again, and a cancel only spares the ticks in between.
func (r *ZoneLayerRegistry) CancelWaiting(key ActionKey) { r.cancelWaiting(key) }

// TakeCancels returns what the zonelayer cancelled in this tick.
func (r *ZoneLayerRegistry) TakeCancels() []Cancel { return r.takeCancels() }

// TakeSelf returns what the zonelayer set for itself in this tick.
func (r *ZoneLayerRegistry) TakeSelf() ([]ZoneLayerAction, []Timed[ZoneLayerAction]) {
	return r.take()
}

// ZoneRegistry is what an action of a zone is given besides the state.
type ZoneRegistry struct {
	Common
	self[ZoneAction]
}

// ToSelf sets an action to run in the next tick of the zone.
func (r *ZoneRegistry) ToSelf(a ZoneAction) { r.toSelf(a) }

// ToSelfAt sets an action to run at a tick of the zone.
func (r *ZoneRegistry) ToSelfAt(tick uint64, a ZoneAction) { r.toSelfAt(r.Tick, tick, a) }

// CancelWaiting ends sooner what waits in the zone: the actions the key names that wait for a
// later tick, or for the next one, and what this tick set before the call. It takes effect when
// the tick is over. It does not reach an action that is in the plan of this tick (Plan.Cancel
// does), nor what such an action sets again after the call, so it is not what stops an action
// that sets itself again: that action decides for itself, from the state, whether it still does
// anything and whether it sets itself again, and a cancel only spares the ticks in between.
func (r *ZoneRegistry) CancelWaiting(key ActionKey) { r.cancelWaiting(key) }

// TakeCancels returns what the zone cancelled in this tick.
func (r *ZoneRegistry) TakeCancels() []Cancel { return r.takeCancels() }

// TakeSelf returns what the zone set for itself in this tick.
func (r *ZoneRegistry) TakeSelf() ([]ZoneAction, []Timed[ZoneAction]) { return r.take() }

// WorldRegistry is what an action of the world is given besides the state.
type WorldRegistry struct {
	Common
	self[WorldAction]
}

// ToSelf sets an action to run in the next tick of the world.
func (r *WorldRegistry) ToSelf(a WorldAction) { r.toSelf(a) }

// ToSelfAt sets an action to run at a tick of the world.
func (r *WorldRegistry) ToSelfAt(tick uint64, a WorldAction) { r.toSelfAt(r.Tick, tick, a) }

// CancelWaiting ends sooner what waits in the world: the actions the key names that wait for a
// later tick, or for the next one, and what this tick set before the call. It takes effect when
// the tick is over. It does not reach an action that is in the plan of this tick (Plan.Cancel
// does), nor what such an action sets again after the call, so it is not what stops an action
// that sets itself again: that action decides for itself, from the state, whether it still does
// anything and whether it sets itself again, and a cancel only spares the ticks in between.
func (r *WorldRegistry) CancelWaiting(key ActionKey) { r.cancelWaiting(key) }

// TakeCancels returns what the world cancelled in this tick.
func (r *WorldRegistry) TakeCancels() []Cancel { return r.takeCancels() }

// TakeSelf returns what the world set for itself in this tick.
func (r *WorldRegistry) TakeSelf() ([]WorldAction, []Timed[WorldAction]) { return r.take() }
