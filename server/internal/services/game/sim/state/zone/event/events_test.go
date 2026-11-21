package event

import (
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
)

func TestEveryEventHasItsOwnIDAndNoneIsZero(t *testing.T) {
	all := []Event{ChatMessageSent{}}
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

func TestAZoneMessageGoesToEveryoneInTheZone(t *testing.T) {
	if got := (ChatMessageSent{}).Audience(); got.Kind != state.EveryoneKind {
		t.Fatalf("the audience is %+v, want everyone", got)
	}
}
