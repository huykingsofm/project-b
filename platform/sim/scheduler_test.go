package sim

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/shared/codec/binary"
)

func names(p *Plan[*toyState], name func(Action[*toyState]) string) []string {
	var out []string
	for {
		e, ok := p.Next(4)
		if !ok {
			return out
		}
		out = append(out, name(e.Action()))
	}
}

func anonymousName(a Action[*toyState]) string { return nameOf(a) }

// at is an action that the scheduler of these tests holds.
func at(name string) Action[*toyState] { return &anonymousAction{name: name} }

func dueNames(s *Scheduler[*toyState], tick uint64) []string {
	var out []string
	for _, a := range s.Due(tick) {
		out = append(out, nameOf(a))
	}
	return out
}

func TestSchedulerRunsAnActionAtItsTickInTheOrderItWasSet(t *testing.T) {
	var s Scheduler[*toyState]
	s.Due(1)
	s.At(4, at("later"))
	s.At(4, at("later-too"))
	s.At(2, at("soon"))

	var got []string
	for tick := uint64(2); tick <= 4; tick++ {
		got = append(got, dueNames(&s, tick)...)
		got = append(got, "|")
	}
	want := []string{"soon", "|", "|", "later", "later-too", "|"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if s.Pending() != 0 {
		t.Fatal("something is still set")
	}
}

func TestSchedulerRunsAnActionForAPastTickAtTheNextOne(t *testing.T) {
	var s Scheduler[*toyState]
	s.Due(5)
	s.At(3, at("late"))
	if got := dueNames(&s, 6); !slices.Equal(got, []string{"late"}) {
		t.Fatalf("got %v", got)
	}
}

func TestSchedulerFindsTheActionOfAnIDThatWaitsForTheEarliestTick(t *testing.T) {
	var s Scheduler[*toyState]
	s.At(9, &authoredAction{name: "late", author: 1})
	s.At(5, &authoredAction{name: "early", author: 2})
	s.At(5, &authoredAction{name: "early too", author: 3})
	if a, ok := s.FindByID(testID); !ok || nameOf(a) != "early" {
		t.Fatalf("found %v, %v, want the first of the earliest tick", a, ok)
	}
	if _, ok := s.FindByID(testID + 1); ok {
		t.Fatal("an action of another ID was found")
	}
	if s.Pending() != 3 {
		t.Fatalf("%d waiting, want 3", s.Pending())
	}
}

func aSchedulerThatWaits() *Scheduler[*toyState] {
	var s Scheduler[*toyState]
	s.At(30, &addAction{N: 10})
	s.Due(4) // the last tick it gave out
	s.At(9, &addAction{N: 12})
	s.At(7, &anonymousAction{name: "shield", priority: -50})
	s.At(9, &addAction{N: -6})
	return &s
}

func hashOf(s *Scheduler[*toyState]) uint64 {
	h := NewHasher()
	s.Hash(&h)
	return h.Sum()
}

func TestASchedulerComesBackAsItWasWritten(t *testing.T) {
	c := mustCodecs(t)
	s := aSchedulerThatWaits()
	var w binary.Writer
	if err := s.Encode(&w); err != nil {
		t.Fatal(err)
	}
	var got Scheduler[*toyState]
	r := binary.NewReader(w.B)
	if err := got.Decode(c, r, 1); err != nil || r.Left() != 0 {
		t.Fatalf("error %v, %d bytes left", err, r.Left())
	}
	if hashOf(&got) != hashOf(s) || got.Pending() != 4 {
		t.Fatalf("what it waits for differs: %d actions, want 4", got.Pending())
	}
	var again binary.Writer
	if err := got.Encode(&again); err != nil || !bytes.Equal(again.B, w.B) {
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
		var cut Scheduler[*toyState]
		if err := cut.Decode(c, binary.NewReader(w.B[:n]), 1); err == nil {
			t.Fatalf("%d of %d bytes were read", n, len(w.B))
		}
	}
}

func TestTheHashOfASchedulerSaysWhatItWaitsFor(t *testing.T) {
	a, b := aSchedulerThatWaits(), aSchedulerThatWaits()
	if hashOf(a) != hashOf(b) {
		t.Fatal("two schedulers that wait for the same have different hashes")
	}
	b.At(11, &addAction{N: 13})
	if hashOf(a) == hashOf(b) {
		t.Fatal("an action more did not change the hash")
	}
	c := aSchedulerThatWaits()
	c.Due(9) // what was set for tick 9 has run
	if hashOf(a) == hashOf(c) {
		t.Fatal("actions that ran did not change the hash")
	}
	d := aSchedulerThatWaits()
	d.At(30, &anonymousAction{name: "a"})
	e := aSchedulerThatWaits()
	e.At(30, &anonymousAction{name: "b"})
	if hashOf(d) == hashOf(e) {
		t.Fatal("a field of the action did not change the hash")
	}
}

func TestASchedulerThatHasATickTwiceIsRefused(t *testing.T) {
	var w binary.Writer
	w.Uvarint(0) // the last tick
	w.Uvarint(2) // two ticks
	for range 2 {
		w.Uvarint(5) // the same tick
		w.Uvarint(0) // no action
	}
	var s Scheduler[*toyState]
	err := s.Decode(mustCodecs(t), binary.NewReader(w.B), 1)
	if !errors.Is(err, binary.ErrBadValue) {
		t.Fatalf("error %v, want a bad value", err)
	}
}
