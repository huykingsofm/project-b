package event

import "testing"

func TestEveryEventHasItsOwnIDAndNoneIsZero(t *testing.T) {
	all := []Event{PrivateChatMessageSent{}}
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
