// Package event holds the events of a zonelayer: what its clients should be told of a tick, in the
// order it happened. A zonelayer's events are its own: the zone and the world define theirs again,
// so the same name in two of them is not a clash.
package event

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// ID names a kind of event of a zonelayer, so that a client or an encoder can tell the kinds apart
// by number. The numbers count up from 1 and 0 is no event. Each event type returns its own, with a
// method ID() in its file.
//
// This is the register of the events of the scope: a new event is added at the end of the list, and
// a number is never changed or reused, because a client may have kept it. The IDs of the three
// scopes are separate types, so an event of one scope cannot be taken for one of another.
type ID uint16

// Event is one thing that happened to the state of a zonelayer in a tick. What an event is made of
// is in the file of each event. The events of a tick are kept in a state.Events[Event].
//
// An event also says who receives it, with Audience: what only one player should hear is never in
// the audience of everyone.
type Event interface {
	ID() ID
	Audience() state.Audience
}

// The IDs of the events.
const (
	CharacterEnteredID ID = iota + 1
	CharacterLeftID
	CharacterNoticedID
	ChatMessageSentID
	EntityDiedID
	EntityHealthChangedID
	EntityMovedID
	ItemAppearedID
	ItemDisappearedID
)
