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

func TestSchedulerRunsAnActionAtItsTickAndCronsEveryPeriod(t *testing.T) {
	var s Scheduler[string]
	s.Due(1)
	s.At(4, "later")
	s.At(4, "later-too")
	s.At(2, "soon")
	s.AddCron(2, "cron")

	var got []string
	for tick := uint64(2); tick <= 4; tick++ {
		got = append(got, s.Due(tick)...)
		got = append(got, "|")
	}
	want := []string{"soon", "cron", "|", "|", "later", "later-too", "cron", "|"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if s.Pending() != 0 {
		t.Fatal("something is still set")
	}
}

func TestSchedulerRunsAnActionForAPastTickAtTheNextOne(t *testing.T) {
	var s Scheduler[string]
	s.Due(5)
	s.At(3, "late")
	if got := s.Due(6); !slices.Equal(got, []string{"late"}) {
		t.Fatalf("got %v", got)
	}
}
