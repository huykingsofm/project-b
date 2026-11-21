package event

import (
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
)

func TestEveryEventHasItsOwnIDAndNoneIsZero(t *testing.T) {
	all := []Event{PrivateChatMessageSent{}, CharacterItemAdded{}}
	seen := map[ID]Event{}
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
	chat := PrivateChatMessageSent{FromID: 1, ToID: 2}.Audience()
	if chat.Kind != state.PlayersKind || !slices.Equal(chat.Players, []state.PlayerID{1, 2}) {
		t.Fatalf("a private message has the audience %+v, want players 1 and 2", chat)
	}
	item := CharacterItemAdded{CharacterID: 5}.Audience()
	if item.Kind != state.CharactersKind || !slices.Equal(item.Characters, []state.CharacterID{5}) {
		t.Fatalf("an item added has the audience %+v, want character 5", item)
	}
}
