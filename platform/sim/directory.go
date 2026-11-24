package sim

// StateStatus is what a StateDirectory says of a state ID.
type StateStatus uint8

// The statuses. The zero value is a state ID that was never given, so a directory that knows
// nothing of an ID says that.
const (
	// StateNotGiven: the ID names no state: it was never given.
	StateNotGiven StateStatus = iota
	// StateLive: the ID is given to a state that exists, whether or not its engine runs.
	StateLive
	// StateRemoved: the ID was given to a state that was removed. An ID is never given again, so it
	// stays removed for good.
	StateRemoved
)

// StateDirectory says what a state ID is: a state that is live, one that was given and removed, or
// one that was never given. Where states come and go, the state that gives their IDs is the
// directory, and a router asks it why a message has no engine to go to.
type StateDirectory interface {
	StateStatus(id StateID) StateStatus
}
