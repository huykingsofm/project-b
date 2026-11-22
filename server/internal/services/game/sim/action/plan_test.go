package action

import (
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/config"
)

func TestSmallerPriorityRunsFirstAndEqualOnesInArrivalOrder(t *testing.T) {
	p := NewPlan[*anonymousAction]()
	p.Append(&anonymousAction{name: "c", priority: 0})
	p.Append(&anonymousAction{name: "a", priority: -10})
	p.Append(&anonymousAction{name: "d", priority: 0})
	p.Append(&anonymousAction{name: "e", priority: 50})
	if got := names(p, anonymousName); !slices.Equal(got, []string{"a", "c", "d", "e"}) {
		t.Fatalf("order = %v", got)
	}
}

func TestWhatIsAddedWhileRunningGoesAfterTheRunningEntry(t *testing.T) {
	p := NewPlan[*anonymousAction]()
	p.Append(&anonymousAction{name: "first", priority: 0})
	p.Append(&anonymousAction{name: "last", priority: 10})
	var got []string
	for {
		e, ok := p.Next()
		if !ok {
			break
		}
		got = append(got, e.Action().name)
		if e.Action().name == "first" {
			p.Append(
				&anonymousAction{name: "early", priority: -100},
			) // cannot run before what already ran
		}
	}
	if !slices.Equal(got, []string{"first", "early", "last"}) {
		t.Fatalf("order = %v", got)
	}
}

func TestFindByIDAndByActorAndCancel(t *testing.T) {
	p := NewPlan[*authoredAction]()
	p.Append(&authoredAction{anonymousAction{"modifier", -50}, 0})
	p.Append(&authoredAction{anonymousAction{"by-1", 0}, 1})
	p.Append(&authoredAction{anonymousAction{"by-2", 0}, 2})
	p.Append(&authoredAction{anonymousAction{"by-2-again", 0}, 2})
	if e, _ := p.Next(); e.Action().name != "modifier" { // the modifier runs
		t.Fatal("unexpected first action")
	}

	if got := p.Find(PlanKey{ID: testID}); len(got) != 3 {
		t.Fatalf("by ID found %d, want 3", len(got))
	}
	byTwo := p.Find(PlanKey{Actor: 2, ID: testID})
	if len(byTwo) != 2 || byTwo[0].Action().name != "by-2" {
		t.Fatalf("by engine found %v", byTwo)
	}
	p.Cancel(byTwo[0])
	if got := names(p, func(a *authoredAction) string { return a.name }); !slices.Equal(
		got, []string{"by-1", "by-2-again"},
	) {
		t.Fatalf("after cancel = %v", got)
	}
	if c := p.Cancelled(); len(c) != 1 || c[0].Action().name != "by-2" {
		t.Fatalf("cancelled = %v", c)
	}
}

func TestAnActionAddedByAnotherIsOneDeeperAndAChainIsCutAtTheMaximum(t *testing.T) {
	cfg, _, err := config.Build(1, map[string]string{"action.max_depth": "2"})
	if err != nil {
		t.Fatal(err)
	}
	p := NewPlan[*anonymousAction]()
	p.Reset(cfg)
	p.Append(&anonymousAction{name: "command"})
	var ran []uint8
	for {
		e, ok := p.Next()
		if !ok {
			break
		}
		ran = append(ran, e.depth)
		p.Append(&anonymousAction{name: "child"}) // every action that runs adds one
	}
	if !slices.Equal(ran, []uint8{0, 1, 2}) {
		t.Fatalf("depths that ran = %v, want [0 1 2]", ran)
	}
	if len(p.TooDeep()) != 1 {
		t.Fatalf("%d entries cut, want 1", len(p.TooDeep()))
	}
}

type typedAction struct{ anonymousAction }

func TestFindTypeReturnsTheActionsOfOneGoTypeThatHaveNotRunAndAreNotCancelled(t *testing.T) {
	p := NewPlan[Action]()
	p.Append(&anonymousAction{name: "a"})
	first := &typedAction{anonymousAction{name: "t1"}}
	second := &typedAction{anonymousAction{name: "t2"}}
	third := &typedAction{anonymousAction{name: "t3", priority: 10}}
	p.Append(first)
	p.Append(second)
	p.Append(third)

	hits := p.FindType[*typedAction]()
	if len(hits) != 3 || hits[0].Action != first || hits[1].Action != second ||
		hits[2].Action != third {
		t.Fatalf(
			"found %d entries, want the three typed actions in the order they were added",
			len(hits),
		)
	}
	if other := p.FindType[*anonymousAction](); len(other) != 1 {
		t.Fatalf("found %d anonymous actions, want 1", len(other))
	}

	p.Cancel(hits[1].Entry)
	p.Next() // runs the first entry in the order of the plan, which is "a" or "t1"
	p.Next()
	if got := p.FindType[*typedAction](); len(got) != 1 || got[0].Action != third {
		t.Fatalf("after one cancelled and two run, found %d, want only the third", len(got))
	}
}
