package sim

import (
	"errors"
	"fmt"
	"sync"

	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// StateID names a state, whatever its type, and so names the engine that owns it and is the address
// of a message. Its 8 high bits are the StateType and its 56 low bits a number that is local to the
// type, so the type of a state is known from its ID and two states of different types never have
// the same ID. A StateID is never given again: the number of a type is counted up and not taken
// back. A thing that lives inside a state, such as an entity, takes an ID of its own that has the
// ID of its state in it, so that it is unique as well (see EntityID in the game).
type StateID uint64

// maxLocalStateID is the greatest number that is local to a type: 2^56 - 1.
const maxLocalStateID = 1<<56 - 1

// NewStateID makes the ID of the state of a type with a number that is local to the type, which
// must be at least 1 and at most 2^56 - 1: a number outside of that is cut, so a caller that counts
// checks it first (see StateIDCounters).
func NewStateID(t StateType, local uint64) StateID {
	return StateID(uint64(t)<<56 | local&maxLocalStateID)
}

// Type is the type of the state that has the ID.
func (id StateID) Type() StateType { return StateType(id >> 56) }

// Local is the number of the state among the states of its type.
func (id StateID) Local() uint64 { return uint64(id) & maxLocalStateID }

// StateType names a kind of state, by a number that the game registers with the codecs: see
// Codecs.RegisterState. A number is never changed or reused, because checkpoints and the replay log
// write it.
type StateType uint8

var (
	typeNamesMu sync.RWMutex
	typeNames   = map[StateType]string{}
	namedTypes  = map[string]StateType{}
)

// MapStateTypeToName gives a type of state its name, in letters, for a file or a log: a log names
// the directory of an engine after it. It is called where the type is defined, once for each type.
// A type has one name and a name belongs to one type, and neither is ever changed, because what is
// written to the disk carries the name; it reports an error for a second name of a type, or a name
// that another type has.
func MapStateTypeToName(t StateType, name string) error {
	if name == "" {
		return fmt.Errorf("the state type %d is given no name", t)
	}
	typeNamesMu.Lock()
	defer typeNamesMu.Unlock()
	if old, ok := typeNames[t]; ok {
		return fmt.Errorf(
			"the state type %d is called %q already, and cannot be called %q", t, old, name,
		)
	}
	if other, ok := namedTypes[name]; ok {
		return fmt.Errorf("the name %q is the name of the state type %d already", name, other)
	}
	typeNames[t] = name
	namedTypes[name] = t
	return nil
}

// String is the name that MapStateTypeToName gave the type, or "state type" and its number when
// it was given none.
func (t StateType) String() string {
	typeNamesMu.RLock()
	defer typeNamesMu.RUnlock()
	if name, ok := typeNames[t]; ok {
		return name
	}
	return fmt.Sprintf("state type %d", uint8(t))
}

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
	// Tick is the tick that the state is at.
	Tick() uint64
	// NextTick takes the state to the next tick, one more than the one it is at, at the start of
	// every tick: the only way its tick changes, so what counts in ticks in it counts once for each
	// tick. The engine checks that Tick says one more after it.
	NextTick()
	// Clone returns a copy of the data that shares nothing with the state, and has no event. The
	// engine publishes it for the other engines to read and never changes it again.
	Clone() State
	// Hash is the hash of the data of the state.
	Hash() uint64
	// Encode writes the state whole, but not its type: Codecs.EncodeState writes that first.
	Encode(w *binary.Writer) error
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
