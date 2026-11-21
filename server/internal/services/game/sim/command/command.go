// Package command holds the commands: what comes into the core from outside, such as what a client
// asked for. A command is data. It names what is asked, and translates itself into the actions of
// the scope it is addressed to; the engine puts those actions in its plan, where they are run like
// any others.
//
// Translating needs no state and cannot fail: whether what is asked is possible is the business of
// the action, which looks at the state when it runs and, if it cannot, changes nothing.
package command

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/action"

// ZoneLayerCommand is a command addressed to a zonelayer.
type ZoneLayerCommand interface {
	TranslateZoneLayer() []action.ZoneLayerAction
}

// ZoneCommand is a command addressed to a zone.
type ZoneCommand interface {
	TranslateZone() []action.ZoneAction
}

// WorldCommand is a command addressed to the world.
type WorldCommand interface {
	TranslateWorld() []action.WorldAction
}
