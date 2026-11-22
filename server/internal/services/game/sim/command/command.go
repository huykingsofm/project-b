// Package command holds the commands: what comes into the core from outside, such as what a client
// asked for. A command is data. It names what is asked, and translates itself into the actions of
// the scope it is addressed to; the engine puts those actions in its plan, where they are run like
// any others.
//
// Translating needs no state and cannot fail: whether what is asked is possible is the business of
// the action, which looks at the state when it runs and, if it cannot, changes nothing.
package command

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// ID names a kind of command, by a number: see id.go for the register. A command returns its own
// with its method ID(). The IDs of the commands of the three scopes are of one type and one
// register, like the IDs of the actions.
type ID uint16

// Command is what the commands of every scope have in common: an ID, and how to write the fields of
// the command as bytes, for the replay log. Reading them back is a function of each command, found
// by its ID (see Decode).
type Command interface {
	ID() ID
	// Encode writes the fields of the command, not its ID: Encode of this package writes the ID
	// first.
	Encode(w *binarycodec.Writer) error
}

// ZoneLayerCommand is a command addressed to a zonelayer.
type ZoneLayerCommand interface {
	Command
	TranslateZoneLayer() []action.ZoneLayerAction
}

// ZoneCommand is a command addressed to a zone.
type ZoneCommand interface {
	Command
	TranslateZone() []action.ZoneAction
}

// WorldCommand is a command addressed to the world.
type WorldCommand interface {
	Command
	TranslateWorld() []action.WorldAction
}
