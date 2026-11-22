package state

import (
	"slices"
)

// AudienceKind says how an Audience names the ones who receive an event.
type AudienceKind uint8

// The kinds of audience. A new kind is added at the end of the list, and a number is never changed
// or reused.
const (
	// EveryoneKind is everyone who watches the scope that the event came from: all the characters
	// of a zonelayer or of a zone, or every player for the world.
	EveryoneKind AudienceKind = iota + 1
	// CharactersKind is a list of characters, which may be one.
	CharactersKind
	// PlayersKind is a list of players, which may be one. It is for what is between players and not
	// between characters, which a character that has no player could not receive.
	PlayersKind
	// InSightOfKind is whoever can see an area of the map. How far an observer sees is decided
	// where the audience is resolved, not by the event.
	InSightOfKind
)

// Audience says who receives an event. The event declares it, and whatever sends events to the
// clients resolves it: the events themselves never name a connection. An audience is meant for the
// ones that can observe the scope that emitted the event, and observers are characters (a character
// that has a player is shown to that player; an NPC may receive the same events to decide what to
// do).
//
// An audience is built with the functions below and never by filling the fields, because only the
// fields of its own kind mean anything.
type Audience struct {
	Kind       AudienceKind
	Characters []CharacterID // for CharactersKind
	Players    []PlayerID    // for PlayersKind
	Area       Rect          // for InSightOfKind
}

// AudienceEveryone is everyone who watches the scope the event came from.
func AudienceEveryone() Audience { return Audience{Kind: EveryoneKind} }

// AudienceCharacters is the given characters, and only them.
func AudienceCharacters(ids ...CharacterID) Audience {
	return Audience{Kind: CharactersKind, Characters: slices.Clone(ids)}
}

// AudiencePlayers is the given players, and only them.
func AudiencePlayers(ids ...PlayerID) Audience {
	return Audience{Kind: PlayersKind, Players: slices.Clone(ids)}
}

// AudienceInSightOf is whoever can see the area.
func AudienceInSightOf(area Rect) Audience { return Audience{Kind: InSightOfKind, Area: area} }
