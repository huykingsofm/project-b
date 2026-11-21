// Package action holds every action of the game, and what an action is made of.
//
// An action is code that changes the state of one scope (the world, a zone or a zonelayer) and never
// leaves it. Each action belongs to exactly one scope, and the name of its type, the name of its
// file and the interface it implements all say which:
//
//   - the type ends with the name of its scope and then Action: ZoneLayerCharacterMoveAction,
//     ZoneChatSendAction, WorldChatPrivateSendAction;
//   - the file is named <scope>_<name>_action.go: zonelayer_character_move_action.go;
//   - it implements the interface of its scope: ZoneLayerAction, ZoneAction or WorldAction.
//
// The ID of an action is a type that the action's own file defines for it (a struct{}), so two
// IDs are never the same by accident and there is no list of them to keep. The same goes for a
// cooldown key.
package action

import (
	worldstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
)

// ID names a kind of action. Its value is a value of a struct{} type that the file of the action
// defines for it.
type ID any

// Acting is implemented by an action that has an author: the actor that performs it. The author
// is a character for an action of a zonelayer or a zone and a player for a private message.
type Acting interface{ Actor() any }

// Action is what the actions of every scope have in common: an ID and a priority. A plan, a
// message, a scheduler and a cron hold actions by it.
type Action interface {
	ID() ID
	Priority() int16
}

// ZoneLayerAction is an action of a zonelayer. It is told what to do and cannot fail: when what it was
// asked for is not possible it changes nothing, and that is its result. It changes the state only
// through the methods of the state, which write the events; what it wants done outside its zonelayer
// (a message to another engine, an action for itself at a later tick) it puts in the registry, and
// the engine does it when the tick is over. Its priority says when it runs among the actions of a
// tick: the smaller runs first, and equal priorities run in the order the actions arrived.
type ZoneLayerAction interface {
	Action
	ApplyZoneLayer(c *Context, s *zonelayerstate.State, r *ZoneLayerRegistry, p *Plan[ZoneLayerAction])
}

// ZoneLayerCancellableAction is a ZoneLayerAction that has something to settle when another action cancels
// it before it runs. It is an action in every other way: it has an ID and a priority.
type ZoneLayerCancellableAction interface {
	ZoneLayerAction
	ApplyZoneLayerCancel(c *Context, s *zonelayerstate.State, r *ZoneLayerRegistry)
}

// ZoneAction is an action of a zone, under the same rules as a ZoneLayerAction.
type ZoneAction interface {
	Action
	ApplyZone(c *Context, s *zonestate.State, r *ZoneRegistry, p *Plan[ZoneAction])
}

// WorldAction is an action of the world, under the same rules as a ZoneLayerAction.
type WorldAction interface {
	Action
	ApplyWorld(c *Context, s *worldstate.State, r *WorldRegistry, p *Plan[WorldAction])
}
