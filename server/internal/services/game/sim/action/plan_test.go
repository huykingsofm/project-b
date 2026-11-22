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

	if got := p.Find(ActionKey{ID: testID}); len(got) != 3 {
		t.Fatalf("by ID found %d, want 3", len(got))
	}
	byTwo := p.Find(ActionKey{Actor: 2, ID: testID})
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

// typedID is the ID of typedAction, and of impostorAction, which is the wrong type for it.
const typedID ID = 60001

type typedAction struct{ anonymousAction }

func (a *typedAction) ID() ID { return typedID }

// impostorAction has the ID of typedAction and is not one: a mistake of whoever made it.
type impostorAction struct{ anonymousAction }

func (a *impostorAction) ID() ID { return typedID }

func typedNames(p *Plan[Action]) []string {
	var out []string
	for _, a := range p.FindByID[*typedAction](typedID) {
		out = append(out, a.name)
	}
	return out
}

func TestFindByIDReturnsTheActionsOfOneIDThatHaveNotRunAndAreNotCancelled(t *testing.T) {
	p := NewPlan[Action]()
	p.Append(&anonymousAction{name: "a"})
	p.Append(&typedAction{anonymousAction{name: "t1"}})
	second := &typedAction{anonymousAction{name: "t2"}}
	p.Append(second)
	p.Append(&typedAction{anonymousAction{name: "t3", priority: 10}})

	if got := typedNames(p); !slices.Equal(got, []string{"t1", "t2", "t3"}) {
		t.Fatalf("found %v, want the three typed actions in the order they were added", got)
	}
	for e, a := range p.FindByID[*typedAction](typedID) {
		if a == second {
			p.Cancel(e)
		}
	}
	p.Next() // runs the first entry in the order of the plan, which is "a" or "t1"
	p.Next()
	if got := typedNames(p); !slices.Equal(got, []string{"t3"}) {
		t.Fatalf("after one cancelled and two run, found %v, want only t3", got)
	}
}

func TestFindByIDIsLazyAndAnEntryCancelledBeforeTheLoopReachesItIsLeftOut(t *testing.T) {
	p := NewPlan[Action]()
	var entries []*PlanEntry[Action]
	for _, name := range []string{"t1", "t2", "t3"} {
		p.Append(&typedAction{anonymousAction{name: name}})
	}
	for e := range p.FindByID[*typedAction](typedID) {
		entries = append(entries, e)
	}
	var seen []string
	for e, a := range p.FindByID[*typedAction](typedID) {
		seen = append(seen, a.name)
		p.Cancel(entries[2]) // t3: not reached yet
		_ = e
	}
	if !slices.Equal(seen, []string{"t1", "t2"}) {
		t.Fatalf("walked %v, want t1 and t2: t3 was cancelled before the loop reached it", seen)
	}
	seen = nil
	for _, a := range p.FindByID[*typedAction](typedID) {
		seen = append(seen, a.name)
		break
	}
	if !slices.Equal(seen, []string{"t1"}) {
		t.Fatalf("a loop that stops walked %v", seen)
	}
}

func TestAnActionOfTheIDButNotOfTheTypeIsLeftOutAndTheOthersAreFound(t *testing.T) {
	p := NewPlan[Action]()
	p.Append(&typedAction{anonymousAction{name: "t1"}})
	p.Append(&impostorAction{anonymousAction{name: "impostor"}})
	p.Append(&typedAction{anonymousAction{name: "t2"}})
	if got := typedNames(p); !slices.Equal(got, []string{"t1", "t2"}) {
		t.Fatalf("found %v, want the two of the right type", got)
	}
}

func TestAWalkInsideAnotherWalkHasItsOwnPlace(t *testing.T) {
	p := NewPlan[Action]()
	for _, name := range []string{"t1", "t2"} {
		p.Append(&typedAction{anonymousAction{name: name}})
	}
	pairs := 0
	for range p.FindByID[*typedAction](typedID) {
		for range p.FindByID[*typedAction](typedID) {
			pairs++
		}
	}
	if pairs != 4 {
		t.Fatalf("%d pairs, want 4", pairs)
	}
}

func TestAWalkOfThePlanMakesNothingToCollect(t *testing.T) {
	p := NewPlan[Action]()
	for range 20 {
		p.Append(&typedAction{})
	}
	walk := func() {
		for range p.FindByID[*typedAction](typedID) {
		}
	}
	if allocs := testing.AllocsPerRun(100, walk); allocs != 0 {
		t.Fatalf("a walk allocates %.0f times, want 0", allocs)
	}
}
