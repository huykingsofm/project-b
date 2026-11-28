package event_test

import (
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/zone/event"
	"github.com/huykingsofm/project-b/shared/testing/aliasing"
)

// allEvents has an event of every kind of the package, for the tests that look at all of them.
var allEvents = []event.Event{event.ChatMessageSent{}, event.ExitAdded{}}

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

func TestAZoneMessageAndAnExitGoToEveryoneInTheZone(t *testing.T) {
	for _, e := range allEvents {
		if got := e.Audience(); got.Kind != state.EveryoneKind {
			t.Fatalf("%T: the audience is %+v, want everyone", e, got)
		}
	}
}

// Events leave the engine to the emitter, which reads them on another goroutine, so they are values
// (R13).
func TestEveryEventIsAValue(t *testing.T) {
	for _, e := range allEvents {
		if err := aliasing.CheckCloneable(reflect.TypeOf(e), aliasing.Rules{}); err != nil {
			t.Errorf("%T: %v", e, err)
		}
	}
}
