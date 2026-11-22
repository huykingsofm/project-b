package action

import (
	"bytes"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

func aSchedulerThatWaits() *Scheduler[ZoneLayerAction] {
	var s Scheduler[ZoneLayerAction]
	s.At(30, &ZoneLayerCharacterRegenAction{Every: 10})
	s.Due(4) // the last tick it gave out
	s.At(9, &ZoneLayerItemEntityRemoveAction{EntityID: 12})
	s.At(7, &ZoneLayerCharacterShieldGrantAction{
		CharacterID: 3, Duration: 20, started: true, until: 31,
	})
	s.At(9, &ZoneLayerCharacterMoveAction{
		CharacterID: 3, Dir: state.East, Ticks: 6, reserved: true,
	})
	return &s
}

func hashOf(s *Scheduler[ZoneLayerAction]) uint64 {
	h := state.NewHasher()
	HashScheduler(&h, s)
	return h.Sum()
}

func TestASchedulerComesBackAsItWasWritten(t *testing.T) {
	s := aSchedulerThatWaits()
	var w binarycodec.Writer
	if err := EncodeScheduler(&w, s); err != nil {
		t.Fatal(err)
	}
	var got Scheduler[ZoneLayerAction]
	r := binarycodec.NewReader(w.B)
	if err := DecodeScheduler(r, 1, &got); err != nil || r.Left() != 0 {
		t.Fatalf("error %v, %d bytes left", err, r.Left())
	}
	if hashOf(&got) != hashOf(s) || got.Pending() != 4 {
		t.Fatalf("what it waits for differs: %d actions, want 4", got.Pending())
	}
	var again binarycodec.Writer
	if err := EncodeScheduler(&again, &got); err != nil || !bytes.Equal(again.B, w.B) {
		t.Fatalf("it is written differently: %v", err)
	}
	// it goes on as the original does: the same actions at the same ticks
	for tick := uint64(5); tick <= 40; tick++ {
		a, b := s.Due(tick), got.Due(tick)
		if len(a) != len(b) {
			t.Fatalf("tick %d: %d actions, want %d", tick, len(b), len(a))
		}
	}
	for n := range w.B { // every proper prefix
		var cut Scheduler[ZoneLayerAction]
		if err := DecodeScheduler(binarycodec.NewReader(w.B[:n]), 1, &cut); err == nil {
			t.Fatalf("%d of %d bytes were read", n, len(w.B))
		}
	}
}

func TestTheHashOfASchedulerSaysWhatItWaitsFor(t *testing.T) {
	a, b := aSchedulerThatWaits(), aSchedulerThatWaits()
	if hashOf(a) != hashOf(b) {
		t.Fatal("two schedulers that wait for the same have different hashes")
	}
	b.At(11, &ZoneLayerItemEntityRemoveAction{EntityID: 13})
	if hashOf(a) == hashOf(b) {
		t.Fatal("an action more did not change the hash")
	}
	c := aSchedulerThatWaits()
	c.Due(9) // what was set for tick 9 has run
	if hashOf(a) == hashOf(c) {
		t.Fatal("actions that ran did not change the hash")
	}
	d := aSchedulerThatWaits()
	d.At(30, &ZoneLayerCharacterShieldGrantAction{CharacterID: 3, Duration: 20, started: false})
	e := aSchedulerThatWaits()
	e.At(30, &ZoneLayerCharacterShieldGrantAction{CharacterID: 3, Duration: 20, started: true})
	if hashOf(d) == hashOf(e) {
		t.Fatal("a field only the action knows (started) did not change the hash")
	}
}
