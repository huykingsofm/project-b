// Commands: what comes into an engine from outside, such as what a client asked for.

package sim

import "github.com/huykingsofm/project-b/shared/binarycodec"

// CommandID names a kind of command, by a number: the game keeps the register. A command returns
// its own with its method ID(). A number is never changed or reused, because the replay log
// writes it.
type CommandID uint16

// BaseCommand is what the commands of every state type have in common: an ID, and how to write the
// fields of the command as bytes, for the replay log. Reading them back is a function of each
// command, found by its ID (see Codecs.DecodeCommand).
type BaseCommand interface {
	ID() CommandID
	// Encode writes the fields of the command, not its ID: EncodeCommand writes the ID first.
	Encode(w *binarycodec.Writer) error
}

// Command is a command addressed to an engine whose state is of type S. A command is data. It names
// what is asked, and translates itself into the actions of S; the engine puts those actions in its
// plan, where they are run like any others.
//
// Translating needs no state and cannot fail: whether what is asked is possible is the business of
// the action, which looks at the state when it runs and, if it cannot, changes nothing.
type Command[S State] interface {
	BaseCommand
	Translate() []Action[S]
	// Clone returns a copy that shares nothing with the command. A command that holds only values
	// returns itself.
	Clone() Command[S]
}
