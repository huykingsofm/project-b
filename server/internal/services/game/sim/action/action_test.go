package action

import (
	"slices"
	"testing"
)

// testID is the ID of the actions of these tests; it is not one of the register.
const testID ID = 60000

// anonymousAction has an ID and a priority and no author.
type anonymousAction struct {
	name     string
	priority int16
}

func (a *anonymousAction) ID() ID          { return testID }
func (a *anonymousAction) Priority() int16 { return a.priority }

// authoredAction has an author too.
type authoredAction struct {
	anonymousAction
	author int
}

func (a *authoredAction) Actor() any { return a.author }

func names[A interface{ Action }](p *Plan[A], name func(A) string) []string {
	var out []string
	for {
		e, ok := p.Next()
		if !ok {
			return out
		}
		out = append(out, name(e.Action()))
	}
}

func anonymousName(a *anonymousAction) string { return a.name }

// at is an action that the scheduler of these tests holds.
func at(name string) *anonymousAction { return &anonymousAction{name: name} }

func dueNames(s *Scheduler[*anonymousAction], tick uint64) []string {
	var out []string
	for _, a := range s.Due(tick) {
		out = append(out, a.name)
	}
	return out
}

func TestSchedulerRunsAnActionAtItsTickInTheOrderItWasSet(t *testing.T) {
	var s Scheduler[*anonymousAction]
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
	var s Scheduler[*anonymousAction]
	s.Due(5)
	s.At(3, at("late"))
	if got := dueNames(&s, 6); !slices.Equal(got, []string{"late"}) {
		t.Fatalf("got %v", got)
	}
}

func TestSchedulerFindsAndCancelsWhatAKeyNames(t *testing.T) {
	var s Scheduler[*authoredAction]
	mine := func(tick uint64, author int) {
		s.At(tick, &authoredAction{name: "x", author: author})
	}
	mine(9, 1)
	mine(5, 2)
	mine(7, 1)
	byOne := ActionKey{Actor: 1, ID: testID}
	if a, ok := s.Find(byOne); !ok || a.author != 1 {
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
	var s Scheduler[*authoredAction]
	of := func(author int, name string) *authoredAction {
		return &authoredAction{name: name, author: author}
	}
	s.At(8, of(1, "waited"))
	s.At(9, of(2, "waited of another"))
	key := ActionKey{Actor: 1, ID: testID}
	next := []*authoredAction{of(1, "next before"), of(1, "next after"), of(2, "next of another")}
	later := []Timed[*authoredAction]{
		{At: 12, Action: of(1, "later before")}, {At: 13, Action: of(1, "later after")},
	}
	cancels := []Cancel{{Key: key, Next: 1, Later: 1}} // the cancel came after the first of each

	kept := SettleSelf(&s, next, later, cancels)
	var names []string
	for _, a := range kept {
		names = append(names, a.name)
	}
	if !slices.Equal(names, []string{"next after", "next of another"}) {
		t.Fatalf("kept for the next tick: %v", names)
	}
	var waiting []string
	for tick := uint64(1); tick <= 13; tick++ {
		for _, a := range s.Due(tick) {
			waiting = append(waiting, a.name)
		}
	}
	if !slices.Equal(waiting, []string{"waited of another", "later after"}) {
		t.Fatalf("waiting in the scheduler: %v", waiting)
	}
}

func TestACancelMadeByAnActionTakesEffectOnWhatWasSetBeforeIt(t *testing.T) {
	var reg ZoneLayerRegistry
	reg.ToSelf(&ZoneLayerCharacterRegenAction{})
	reg.CancelWaiting(ActionKey{ID: ZoneLayerCharacterRegenActionID})
	reg.ToSelf(&ZoneLayerCharacterRegenAction{Every: 5}) // set after: it stays
	next, later := reg.TakeSelf()
	var s Scheduler[ZoneLayerAction]
	kept := SettleSelf(&s, next, later, reg.TakeCancels())
	if len(kept) != 1 || kept[0].(*ZoneLayerCharacterRegenAction).Every != 5 {
		t.Fatalf("kept %+v, want only the action set after the cancel", kept)
	}
	if more := reg.TakeCancels(); more != nil {
		t.Fatal("the cancels were not taken")
	}
}
