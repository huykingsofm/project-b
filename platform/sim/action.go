package sim

import "github.com/huykingsofm/project-b/shared/codec/binary"

// ActionID names a kind of action, by a number: the game keeps the register. An action returns its
// own with its method ID(). A number is never changed or reused, because the event log writes it.
type ActionID uint16

// Encoder is what an action has to be written: its fields, without its ID.
type Encoder interface {
	Encode(w *binary.Writer)
}

// BaseAction is what the actions of every state type have in common: an ID, a priority and an
// actor. The codecs, which do not know the type of state an action is for, hold actions by it.
type BaseAction interface {
	ID() ActionID
	Priority() int16
	// Actor is the author of the action, the actor that performs it (a character, a player), or nil
	// for an action that has none. It is compared with ==, as an ActionKey does, so it is of a type
	// that can be compared: a slice, a map or a func makes the comparison panic.
	Actor() any
}

// Action is an action of a state of type S: code that is told what to do, changes the state only
// through the methods of the state, and cannot fail: when what it was asked for is not possible it
// changes nothing, and that is its result. What it wants done outside its state (a message to
// another engine, an action for itself at a later tick) it puts in the registry, and the engine
// does it when the tick is over. Its priority says when it runs among the actions of a tick: the
// smaller runs first, and equal priorities run in the order the actions arrived.
//
// An action of one type of state cannot be given to an engine of another, and a message to a state
// carries only actions of that state's type: the compiler checks both.
//
// An action is a value type, with its methods on the value (R29): no one changes it through the
// interface that holds it. One that changes what it holds as it runs (a run of movement that
// reserved its time) changes its own copy and sets that copy for a later tick with the registry.
type Action[S State] interface {
	BaseAction
	Apply(c *Context, s S, r *Registry[S], p *Plan[S])
	// Clone returns a copy that shares nothing with the action: the action itself, or a copy of
	// what it holds that is not a value (a character). A checkpoint, and an engine that takes a
	// message, copy actions with it.
	Clone() Action[S]
}

// Cancellable is an Action that has something to settle when another action cancels it before it
// runs. It is an action in every other way: it has an ID and a priority.
type Cancellable[S State] interface {
	Action[S]
	ApplyCancel(c *Context, s S, r *Registry[S])
}
