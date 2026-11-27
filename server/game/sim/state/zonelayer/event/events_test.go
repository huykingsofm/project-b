package event_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/zonelayer/event"
	"github.com/huykingsofm/project-b/shared/testing/aliasing"
)

// allEvents has an event of every kind of the package, for the tests that look at all of them.
var allEvents = []event.Event{
	event.CharacterEntered{},
	event.CharacterLeft{},
	event.ChatMessageSent{},
	event.EntityDied{},
	event.EntityHealthChanged{},
	event.EntityMoved{},
	event.ItemAppeared{},
	event.ItemDisappeared{},
}

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

func TestEveryEventSaysWhoReceivesItAndWhatIsPrivateIsNotPublic(t *testing.T) {
	at, to := state.Vec{X: 1000, Y: 2000}, state.Vec{X: 3000, Y: 1000}
	t.Run("a body of a radius is seen when any part of it is in sight", func(t *testing.T) {
		got := event.EntityMoved{From: at, To: to, Radius: 500}.Audience().Area
		want := state.Rect{Min: state.Vec{X: 500, Y: 500}, Max: state.Vec{X: 3500, Y: 2500}}
		if got != want {
			t.Fatalf("the area is %+v, want %+v", got, want)
		}
		for _, e := range []event.Event{
			event.CharacterEntered{Pos: at, Radius: 500},
			event.CharacterLeft{At: at, Radius: 500},
			event.EntityDied{At: at, Radius: 500},
			event.EntityHealthChanged{At: at, Radius: 500},
		} {
			want := state.Rect{Min: state.Vec{X: 500, Y: 1500}, Max: state.Vec{X: 1500, Y: 2500}}
			if got := e.Audience().Area; got != want {
				t.Fatalf("%T has the area %+v, want %+v", e, got, want)
			}
		}
	})
	for _, c := range []struct {
		event event.Event
		want  state.Audience
	}{
		{event.CharacterEntered{Pos: at}, state.AudienceInSightOf(state.RectCovering(at, at))},
		{event.CharacterLeft{At: at}, state.AudienceInSightOf(state.RectCovering(at, at))},
		{event.ChatMessageSent{}, state.AudienceEveryone()},
		{event.EntityDied{At: at}, state.AudienceInSightOf(state.RectCovering(at, at))},
		{event.EntityHealthChanged{At: at}, state.AudienceInSightOf(state.RectCovering(at, at))},
		{
			event.EntityMoved{From: at, To: to},
			state.AudienceInSightOf(
				state.Rect{Min: state.Vec{X: 1000, Y: 1000}, Max: state.Vec{X: 3000, Y: 2000}},
			),
		},
		{event.ItemAppeared{Pos: at}, state.AudienceInSightOf(state.RectCovering(at, at))},
		{event.ItemDisappeared{At: at}, state.AudienceInSightOf(state.RectCovering(at, at))},
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
		rules := aliasing.Rules{Boundary: true}
		if err := aliasing.CheckCloneable(reflect.TypeOf(e), rules); err != nil {
			t.Errorf("%T: %v", e, err)
		}
	}
}
