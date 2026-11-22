package event

import (
	"reflect"
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/shared/aliasing"
)

// allEvents has an event of every kind of the package, for the tests that look at all of them.
var allEvents = []Event{
	CharacterEntered{},
	CharacterLeft{},
	CharacterNoticed{},
	ChatMessageSent{},
	EntityDied{},
	EntityHealthChanged{},
	EntityMoved{},
	ItemAppeared{},
	ItemDisappeared{},
}

func TestEveryEventHasItsOwnIDAndNoneIsZero(t *testing.T) {
	all := allEvents
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

func TestEveryEventSaysWhoReceivesItAndWhatIsPrivateIsNotPublic(t *testing.T) {
	at, to := state.Vec{X: 1000, Y: 2000}, state.Vec{X: 3000, Y: 1000}
	t.Run("a body of a radius is seen when any part of it is in sight", func(t *testing.T) {
		got := EntityMoved{From: at, To: to, Radius: 500}.Audience().Area
		want := state.Rect{Min: state.Vec{X: 500, Y: 500}, Max: state.Vec{X: 3500, Y: 2500}}
		if got != want {
			t.Fatalf("the area is %+v, want %+v", got, want)
		}
		for _, e := range []Event{
			CharacterEntered{Pos: at, Radius: 500},
			CharacterLeft{At: at, Radius: 500},
			EntityDied{At: at, Radius: 500},
			EntityHealthChanged{At: at, Radius: 500},
		} {
			want := state.Rect{Min: state.Vec{X: 500, Y: 1500}, Max: state.Vec{X: 1500, Y: 2500}}
			if got := e.Audience().Area; got != want {
				t.Fatalf("%T has the area %+v, want %+v", e, got, want)
			}
		}
	})
	for _, c := range []struct {
		event Event
		want  state.Audience
	}{
		{CharacterEntered{Pos: at}, state.AudienceInSightOf(state.RectCovering(at, at))},
		{CharacterLeft{At: at}, state.AudienceInSightOf(state.RectCovering(at, at))},
		{CharacterNoticed{CharacterID: 7}, state.AudienceCharacters(7)},
		{ChatMessageSent{}, state.AudienceEveryone()},
		{EntityDied{At: at}, state.AudienceInSightOf(state.RectCovering(at, at))},
		{EntityHealthChanged{At: at}, state.AudienceInSightOf(state.RectCovering(at, at))},
		{
			EntityMoved{From: at, To: to},
			state.AudienceInSightOf(
				state.Rect{Min: state.Vec{X: 1000, Y: 1000}, Max: state.Vec{X: 3000, Y: 2000}},
			),
		},
		{ItemAppeared{Pos: at}, state.AudienceInSightOf(state.RectCovering(at, at))},
		{ItemDisappeared{At: at}, state.AudienceInSightOf(state.RectCovering(at, at))},
	} {
		got := c.event.Audience()
		if got.Kind == 0 || got.Kind != c.want.Kind || got.Area != c.want.Area ||
			!slices.Equal(got.Characters, c.want.Characters) {
			t.Errorf("%T has the audience %+v, want %+v", c.event, got, c.want)
		}
	}
}

// Events leave the engine to the emitter, which reads them on another goroutine, so they are values
// (R29).
func TestEveryEventIsAValue(t *testing.T) {
	for _, e := range allEvents {
		// CharacterNoticed holds the arguments of a text; the state copies them when it makes the
		// event
		rules := aliasing.Rules{Boundary: true, Skip: []string{"event.CharacterNoticed.Args"}}
		if err := aliasing.CheckCloneable(reflect.TypeOf(e), rules); err != nil {
			t.Errorf("%T: %v", e, err)
		}
	}
}
