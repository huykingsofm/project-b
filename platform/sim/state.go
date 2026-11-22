package sim

import (
	"errors"

	"github.com/huykingsofm/project-b/shared/binarycodec"
)

// StateID names a state, whatever its type, and so names the engine that owns it and is the address
// of a message. It is unique among all the states: two states of different types never have the
// same ID. A thing that lives inside a state, such as an entity, takes an ID of its own that has
// the ID of its state in it, so that it is unique as well (see EntityID in the game).
type StateID uint64

// StateType names a kind of state, by a number that the game registers with the codecs: see
// Codecs.RegisterState. A number is never changed or reused, because checkpoints and the replay log
// write it.
type StateType uint8

// Event is something that happened in a state in a tick, for the clients. What an event is made of
// belongs to the game; the engine only hands the events of a tick to its emitter.
type Event any

// State is the state of one engine: the data that the engine owns and is the only one that changes.
// The engine runs the actions of its state, publishes a Snapshot of it at the end of every tick for
// the other engines to read, and writes it to a checkpoint.
//
// A state is data and nothing else: it holds no function, channel or connection, and it never
// reaches another engine. A state of another engine is found by its StateID in a StableView.
type State interface {
	// ID is the ID of the state.
	ID() StateID
	// Type is the type of the state. It must return the same number for every value, including a
	// nil pointer, because the type of a state is asked of its Go type alone (see StateTypeOf).
	Type() StateType
	// String is the name of the type of the state, in letters, for a file or a log. It does not
	// include the ID.
	String() string
	// Tick is the tick that the state is at.
	Tick() uint64
	// SetTick sets the tick that the state is at, at the start of every tick.
	SetTick(tick uint64)
	// Clone returns a copy of the data that shares nothing with the state, and has no event. The
	// engine publishes it for the other engines to read and never changes it again.
	Clone() State
	// Hash is the hash of the data of the state.
	Hash() uint64
	// Encode writes the state whole, but not its type: Codecs.EncodeState writes that first.
	Encode(w *binarycodec.Writer) error
	// TakeEvents returns the events of the tick and empties them.
	TakeEvents() []Event
}

// StateTypeOf is the type of the states of the Go type S: it is asked of the zero value of S, so
// the Type of a state must not read the state.
func StateTypeOf[S State]() StateType {
	var zero S
	return zero.Type()
}

// ErrStateNotFound is the error of a state that no engine published, or that was removed.
var ErrStateNotFound = errors.New("sim: no engine published a state with that ID")

// ErrStateWrongType is the error of a state that was found but is not of the Go type that was asked
// for.
var ErrStateWrongType = errors.New("sim: the state is not of the type asked for")
