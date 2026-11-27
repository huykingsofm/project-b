package event_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/world/event"
	"github.com/huykingsofm/project-b/shared/testing/aliasing"
)

// allEvents has an event of every kind of the package, for the tests that look at all of them.
var allEvents = []event.Event{event.PrivateChatMessageSent{}, event.CharacterItemAdded{}}

func TestEveryEventHasItsOwnIDAndNoneIsZero(t *testing.T) {
	all := allEvents
	seen := map[event.ID]event.Event{}
	for _, e := range all {
		if e.ID() == 0 {
			t.Fatalf("%T has the ID 0", e)
		}
		if other, ok := seen[e.ID()]; ok {
			t.Fatalf("%T and %T have the same ID %d", e, other, e.ID())
		}
		seen[e.ID()] = e
	}
}

func TestPrivateEventsGoOnlyToTheOnesTheyAreAbout(t *testing.T) {
	chat := event.PrivateChatMessageSent{FromID: 1, ToID: 2}.Audience()
	if chat.Kind != state.PlayersKind || !slices.Equal(chat.Players, []state.PlayerID{1, 2}) {
		t.Fatalf("a private message has the audience %+v, want players 1 and 2", chat)
	}
	item := event.CharacterItemAdded{CharacterID: 5}.Audience()
	if item.Kind != state.CharactersKind || !slices.Equal(item.Characters, []state.CharacterID{5}) {
		t.Fatalf("an item added has the audience %+v, want character 5", item)
	}
}

// Events leave the engine to the emitter, which reads them on another goroutine, so they are values
// (R29).
func TestEveryEventIsAValue(t *testing.T) {
	for _, e := range allEvents {
		if err := aliasing.CheckCloneable(reflect.TypeOf(e), aliasing.Rules{}); err != nil {
			t.Errorf("%T: %v", e, err)
		}
	}
}
