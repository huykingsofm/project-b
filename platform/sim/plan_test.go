package sim

import (
	"maps"
	"slices"
	"testing"
)

func TestSmallerPriorityRunsFirstAndEqualOnesInArrivalOrder(t *testing.T) {
	p := NewPlan[*toyState]()
	p.Append(anonymousAction{name: "c", priority: 0})
	p.Append(anonymousAction{name: "a", priority: -10})
	p.Append(anonymousAction{name: "d", priority: 0})
	p.Append(anonymousAction{name: "e", priority: 50})
	if got := names(p, anonymousName); !slices.Equal(got, []string{"a", "c", "d", "e"}) {
		t.Fatalf("order = %v", got)
	}
}

func TestWhatIsAddedWhileRunningGoesAfterTheRunningEntry(t *testing.T) {
	p := NewPlan[*toyState]()
	p.Append(anonymousAction{name: "first", priority: 0})
	p.Append(anonymousAction{name: "last", priority: 10})
	var got []string
	for {
		e, ok := p.Next(4)
		if !ok {
			break
		}
		got = append(got, nameOf(e.Action()))
		if nameOf(e.Action()) == "first" {
			p.Append(
				anonymousAction{name: "early", priority: -100},
			) // cannot run before what already ran
		}
	}
	if !slices.Equal(got, []string{"first", "early", "last"}) {
		t.Fatalf("order = %v", got)
	}
}

func TestFindByIDAndCancel(t *testing.T) {
	p := NewPlan[*toyState]()
	p.Append(&authoredAction{anonymousAction{"modifier", -50}, 0})
	p.Append(&authoredAction{anonymousAction{"by-1", 0}, 1})
	p.Append(&authoredAction{anonymousAction{"by-2", 0}, 2})
	p.Append(&authoredAction{anonymousAction{"by-2-again", 0}, 2})
	if e, _ := p.Next(4); nameOf(e.Action()) != "modifier" { // the modifier runs
		t.Fatal("unexpected first action")
	}

	var got, byTwo []*PlanEntry[*toyState]
	for e, a := range p.FindByID[*authoredAction](testID) {
		got = append(got, e)
		if a.Actor() == 2 {
			byTwo = append(byTwo, e)
		}
	}
	if len(got) != 4 || got[0].Status() != EntryRan || got[1].Status() != EntryPending {
		t.Fatalf("by ID found %d, want 4: the modifier that ran, and the 3 still to run", len(got))
	}
	if len(byTwo) != 2 || nameOf(byTwo[0].Action()) != "by-2" {
		t.Fatalf("by actor found %v", byTwo)
	}
	p.Cancel(byTwo[0])
	if byTwo[0].Status() != EntryCancelled {
		t.Fatal("the status of a cancelled entry is not EntryCancelled")
	}
	if got := names(p, anonymousName); !slices.Equal(
		got, []string{"by-1", "by-2-again"},
	) {
		t.Fatalf("after cancel = %v", got)
	}
	if c := p.Cancelled(); len(c) != 1 || nameOf(c[0].Action()) != "by-2" {
		t.Fatalf("cancelled = %v", c)
	}
}

func TestAnActionAddedByAnotherIsOneDeeperAndAChainIsCutAtTheMaximum(t *testing.T) {
	p := NewPlan[*toyState]()
	p.Append(anonymousAction{name: "command"})
	var ran []uint8
	for {
		e, ok := p.Next(2)
		if !ok {
			break
		}
		ran = append(ran, e.depth)
		p.Append(anonymousAction{name: "child"}) // every action that runs adds one
	}
	if !slices.Equal(ran, []uint8{0, 1, 2}) {
		t.Fatalf("depths that ran = %v, want [0 1 2]", ran)
	}
	cut := 0
	for _, e := range p.queue.All() {
		if e.Status() == EntryCut {
			cut++
		}
	}
	if cut != 1 {
		t.Fatalf("%d entries cut, want 1", cut)
	}
}

// typedID is the ID of typedAction, and of impostorAction, which is the wrong type for it.
const typedID ActionID = 60001

type typedAction struct{ anonymousAction }

func (a *typedAction) ID() ActionID { return typedID }

// impostorAction has the ID of typedAction and is not one: a mistake of whoever made it.
type impostorAction struct{ anonymousAction }

func (a *impostorAction) ID() ActionID { return typedID }

// typedNames is the name of every typed action in the plan, and the status of its entry.
func typedNames(p *Plan[*toyState]) map[string]EntryStatus {
	out := map[string]EntryStatus{}
	for e, a := range p.FindByID[*typedAction](typedID) {
		out[a.name] = e.Status()
	}
	return out
}

// pendingTypedNames is the names of the typed actions still to run, as a caller that changes
// actions has to find them: by checking the status of each entry.
func pendingTypedNames(p *Plan[*toyState]) []string {
	var out []string
	for e, a := range p.FindByID[*typedAction](typedID) {
		if e.Status() == EntryPending {
			out = append(out, a.name)
		}
	}
	return out
}

func TestFindByIDReturnsTheActionsOfOneIDWhateverTheStatusOfTheirEntries(t *testing.T) {
	p := NewPlan[*toyState]()
	p.Append(anonymousAction{name: "a"})
	p.Append(&typedAction{anonymousAction{name: "t1"}})
	second := &typedAction{anonymousAction{name: "t2"}}
	p.Append(second)
	p.Append(&typedAction{anonymousAction{name: "t3", priority: 10}})

	if got := pendingTypedNames(p); !slices.Equal(got, []string{"t1", "t2", "t3"}) {
		t.Fatalf("found %v, want the three typed actions in the order they were added", got)
	}
	for e, a := range p.FindByID[*typedAction](typedID) {
		if a == second {
			p.Cancel(e)
		}
	}
	p.Next(4) // runs the first entry in the order of the plan, which is "a" or "t1"
	p.Next(4)
	want := map[string]EntryStatus{"t1": EntryRan, "t2": EntryCancelled, "t3": EntryPending}
	if got := typedNames(p); !maps.Equal(got, want) {
		t.Fatalf("found %v, want %v", got, want)
	}
	if got := pendingTypedNames(p); !slices.Equal(got, []string{"t3"}) {
		t.Fatalf("still to run %v, want only t3", got)
	}
}

func TestFindByIDIsLazyAndAnEntryCancelledBeforeTheLoopReachesItIsGivenAsCancelled(t *testing.T) {
	p := NewPlan[*toyState]()
	var entries []*PlanEntry[*toyState]
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
		if a.name == "t3" && e.Status() != EntryCancelled {
			t.Fatal("t3 was cancelled before the loop reached it and is not given as cancelled")
		}
	}
	if !slices.Equal(seen, []string{"t1", "t2", "t3"}) {
		t.Fatalf("walked %v, want all three", seen)
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
	p := NewPlan[*toyState]()
	p.Append(&typedAction{anonymousAction{name: "t1"}})
	p.Append(&impostorAction{anonymousAction{name: "impostor"}})
	p.Append(&typedAction{anonymousAction{name: "t2"}})
	if got := pendingTypedNames(p); !slices.Equal(got, []string{"t1", "t2"}) {
		t.Fatalf("found %v, want the two of the right type", got)
	}
}

func TestAWalkInsideAnotherWalkHasItsOwnPlace(t *testing.T) {
	p := NewPlan[*toyState]()
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
	p := NewPlan[*toyState]()
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

func TestBuildGivesTheOrderOfAnAppendOfEachActionAndTheEntriesAreAtDepthZero(t *testing.T) {
	actions := func() []Action[*toyState] {
		return []Action[*toyState]{
			anonymousAction{name: "c", priority: 0}, anonymousAction{name: "a", priority: -10},
			anonymousAction{name: "d", priority: 0}, anonymousAction{name: "e", priority: 50},
			anonymousAction{name: "b", priority: -10},
		}
	}
	one := NewPlan[*toyState]()
	for _, a := range actions() {
		one.Append(a)
	}
	all := NewPlan[*toyState]()
	all.Build(actions())
	a := names(one, anonymousName)
	b := names(all, anonymousName)
	if !slices.Equal(a, b) || !slices.Equal(a, []string{"a", "b", "c", "d", "e"}) {
		t.Fatalf("one by one %v, all at once %v", a, b)
	}
	built := NewPlan[*toyState]()
	built.Build(actions())
	for {
		e, ok := built.Next(4)
		if !ok {
			break
		}
		if e.depth != 0 {
			t.Fatalf("an entry of Build is at depth %d, want 0", e.depth)
		}
	}
}

func TestWhatIsAppendedAfterBuildIsOrderedAmongTheEntriesStillToRunAndFindSeesAll(
	t *testing.T,
) {
	p := NewPlan[*toyState]()
	p.Build([]Action[*toyState]{
		&typedAction{anonymousAction{name: "t1", priority: 0}},
		&typedAction{anonymousAction{name: "t2", priority: 10}},
	})
	e, _ := p.Next(4) // t1 runs
	p.Append(&typedAction{anonymousAction{name: "early", priority: -100}})
	if got := pendingTypedNames(p); !slices.Equal(got, []string{"t2", "early"}) {
		t.Fatalf("found %v, want what is still to run, in the order they were added", got)
	}
	if got := typedNames(p); len(got) != 3 || got["t1"] != EntryRan {
		t.Fatalf("found %v, want the three, t1 among them as ran", got)
	}
	if next, _ := p.Next(4); nameOf(next.Action()) != "early" || next.depth != e.depth+1 {
		t.Fatalf(
			"the next to run is %q at depth %d, want early, one deeper than t1",
			nameOf(next.Action()), next.depth,
		)
	}
}

func TestCancelledListsTheEntriesCancelledInTheOrderTheyWereAndNotTheOnesThatRan(t *testing.T) {
	p := NewPlan[*toyState]()
	p.Build([]Action[*toyState]{
		&typedAction{anonymousAction{name: "t1"}}, &typedAction{anonymousAction{name: "t2"}},
		&typedAction{anonymousAction{name: "t3"}},
	})
	first, _ := p.Next(4)
	p.Cancel(first) // it ran already: nothing to cancel
	var entries []*PlanEntry[*toyState]
	for e := range p.FindByID[*typedAction](typedID) {
		entries = append(entries, e)
	}
	p.Cancel(entries[1])
	p.Cancel(entries[2])
	got := p.Cancelled()
	if len(got) != 2 || nameOf(got[0].Action()) != "t2" || nameOf(got[1].Action()) != "t3" {
		t.Fatalf("cancelled %v, want t2 then t3", got)
	}
}
