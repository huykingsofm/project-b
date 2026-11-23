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
		e, ok := p.Next()
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

func TestSchedulerFindsAndCancelsWhatAKeyNames(t *testing.T) {
	var s Scheduler[*toyState]
	mine := func(tick uint64, author int) {
		s.At(tick, &authoredAction{name: "x", author: author})
	}
	mine(9, 1)
	mine(5, 2)
	mine(7, 1)
	byOne := ActionKey{Actor: 1, ID: testID}
	if a, ok := s.Find(byOne); !ok || a.(*authoredAction).author != 1 {
		t.Fatalf("found %+v, %v", a, ok)
	}
	if _, ok := s.Find(ActionKey{Actor: 3, ID: testID}); ok {
		t.Fatal("an action of an actor that has none was found")
	}
	if _, ok := s.Find(ActionKey{ID: testID + 1}); ok {
		t.Fatal("an action of another ID was found")
	}
	if n := s.Cancel(byOne); n != 2 || s.Pending() != 1 {
		t.Fatalf("%d cancelled, %d left, want 2 and 1", n, s.Pending())
	}
	if n := s.Cancel(ActionKey{ID: testID}); n != 1 || s.Pending() != 0 {
		t.Fatalf("a key with no actor names every action of the ID: %d cancelled", n)
	}
}

func TestACancelTakesAwayWhatWaitedAndWhatWasSetBeforeItAndNotWhatWasSetAfter(t *testing.T) {
	var s Scheduler[*toyState]
	of := func(author int, name string) Action[*toyState] {
		return &authoredAction{name: name, author: author}
	}
	s.At(8, of(1, "waited"))
	s.At(9, of(2, "waited of another"))
	key := ActionKey{Actor: 1, ID: testID}
	next := []Action[*toyState]{of(1, "next before"), of(1, "next after"), of(2, "next of another")}
	later := []Timed[*toyState]{
		{At: 12, Action: of(1, "later before")}, {At: 13, Action: of(1, "later after")},
	}
	cancels := []Cancel{{Key: key, Next: 1, Later: 1}} // the cancel came after the first of each

	kept := s.Settle(next, later, cancels)
	var names []string
	for _, a := range kept {
		names = append(names, nameOf(a))
	}
	if !slices.Equal(names, []string{"next after", "next of another"}) {
		t.Fatalf("kept for the next tick: %v", names)
	}
	var waiting []string
	for tick := uint64(1); tick <= 13; tick++ {
		for _, a := range s.Due(tick) {
			waiting = append(waiting, nameOf(a))
		}
	}
	if !slices.Equal(waiting, []string{"waited of another", "later after"}) {
		t.Fatalf("waiting in the scheduler: %v", waiting)
	}
}

func TestACancelMadeByAnActionTakesEffectOnWhatWasSetBeforeIt(t *testing.T) {
	var reg Registry[*toyState]
	reg.ToSelf(&addAction{})
	reg.CancelWaiting(ActionKey{ID: addID})
	reg.ToSelf(&addAction{N: 5}) // set after: it stays
	next, later := reg.TakeSelf()
	var s Scheduler[*toyState]
	kept := s.Settle(next, later, reg.TakeCancels())
	if len(kept) != 1 || kept[0].(*addAction).N != 5 {
		t.Fatalf("kept %+v, want only the action set after the cancel", kept)
	}
	if more := reg.TakeCancels(); more != nil {
		t.Fatal("the cancels were not taken")
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
