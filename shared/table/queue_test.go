package table_test

import (
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/shared/table"
)

type job struct {
	name     string
	priority int
	kind     string
}

func newJobs(t testing.TB) *table.PriorityQueue[job, int] {
	t.Helper()
	q := table.NewPriorityQueue(func(j job) int { return j.priority })
	if err := q.Index(func(j job) (kind, bool) { return kind(j.kind), true }); err != nil {
		t.Fatal(err)
	}
	return q
}

func namesOfQueue(q *table.PriorityQueue[job, int]) []string {
	var out []string
	for {
		_, j, ok := q.Next()
		if !ok {
			return out
		}
		out = append(out, j.name)
	}
}

func TestTheSmallestPriorityIsTakenFirstAndTheSameOnesInTheOrderTheyCame(t *testing.T) {
	q := newJobs(t)
	q.Add(job{"c", 0, "x"})
	q.Add(job{"a", -10, "x"})
	q.Add(job{"d", 0, "x"})
	q.Add(job{"e", 50, "x"})
	if got := namesOfQueue(q); !slices.Equal(got, []string{"a", "c", "d", "e"}) {
		t.Fatalf("order %v", got)
	}
}

func TestBuildFromGivesTheOrderOfAddingThemOneByOne(t *testing.T) {
	jobs := []job{{"c", 0, "x"}, {"a", -10, "x"}, {"d", 0, "x"}, {"e", 50, "x"}, {"b", -10, "x"}}
	one := newJobs(t)
	for _, j := range jobs {
		one.Add(j)
	}
	all := newJobs(t)
	all.BuildFrom(jobs)
	a, b := namesOfQueue(one), namesOfQueue(all)
	if !slices.Equal(a, b) || !slices.Equal(a, []string{"a", "b", "c", "d", "e"}) {
		t.Fatalf("one by one %v, all at once %v", a, b)
	}
}

func TestWhatIsAddedWhileTakingIsOrderedAmongTheOnesStillToBeTakenNeverBeforeOneTaken(
	t *testing.T,
) {
	q := newJobs(t)
	q.Add(job{"first", 0, "x"})
	q.Add(job{"last", 10, "x"})
	var got []string
	for {
		_, j, ok := q.Next()
		if !ok {
			break
		}
		got = append(got, j.name)
		if j.name == "first" {
			q.Add(job{"early", -100, "x"}) // smaller than all: the next one, not before "first"
		}
	}
	if !slices.Equal(got, []string{"first", "early", "last"}) {
		t.Fatalf("order %v", got)
	}
}

func TestAllWalksEveryElementAndPendingStartsAtTheNextOneToBeTaken(t *testing.T) {
	q := newJobs(t)
	q.BuildFrom([]job{{"a", 1, "x"}, {"b", 2, "x"}, {"c", 3, "x"}, {"d", 4, "x"}})
	q.Next()            // a
	q.Next()            // b
	k, _, _ := q.Next() // c
	_ = k
	q.Cancel(3) // d is cancelled
	names := func(seq func(func(int64, job) bool)) (out []string) {
		for _, j := range seq {
			out = append(out, j.name)
		}
		return out
	}
	if got := names(q.All()); !slices.Equal(got, []string{"a", "b", "c", "d"}) {
		t.Fatalf("All %v, want every element, the ones taken and cancelled included", got)
	}
	if got := names(q.Pending()); len(got) != 0 {
		t.Fatalf("Pending %v, want none: the rest is taken or cancelled", got)
	}
	r := newJobs(t)
	r.BuildFrom([]job{{"a", 1, "x"}, {"b", 2, "x"}, {"c", 3, "x"}})
	r.Next()
	if got := names(r.Pending()); !slices.Equal(got, []string{"b", "c"}) {
		t.Fatalf("Pending %v, want [b c]", got)
	}
}

func TestACancelledElementIsNotTakenAndTheKeysCancelledAreKnown(t *testing.T) {
	q := newJobs(t)
	q.BuildFrom([]job{{"a", 1, "x"}, {"b", 2, "x"}, {"c", 3, "x"}})
	if !q.Cancel(1) || q.Cancel(1) || q.Cancel(9) || q.Cancel(-1) {
		t.Fatal("Cancel did not say whether the element was still to be taken")
	}
	if got := namesOfQueue(q); !slices.Equal(got, []string{"a", "c"}) {
		t.Fatalf("taken %v, want [a c]", got)
	}
	if q.Cancel(0) {
		t.Fatal("an element that was taken was cancelled")
	}
	if got := q.Cancelled(); !slices.Equal(got, []int64{1}) {
		t.Fatalf("cancelled %v, want [1]", got)
	}
	if j, ok := q.Get(1); !ok || j.name != "b" {
		t.Fatal("a cancelled element is not there to be read")
	}
}

func TestLookupFindsOnlyWhatIsStillToBeTakenAndSeesACancelInTheWalk(t *testing.T) {
	q := newJobs(t)
	q.BuildFrom([]job{{"a", 1, "item"}, {"b", 2, "item"}, {"c", 3, "body"}, {"d", 4, "item"}})
	q.Next() // a is taken
	var seen []string
	for k, j := range q.Lookup(kind("item")) {
		seen = append(seen, j.name)
		if j.name == "b" {
			q.Cancel(k + 2) // d, not reached yet: left out
		}
	}
	if !slices.Equal(seen, []string{"b"}) {
		t.Fatalf("walked %v, want [b]: a was taken, d was cancelled before it was reached", seen)
	}
}

func TestResetTakesEverythingOutAndTheQueueGoesOn(t *testing.T) {
	q := newJobs(t)
	q.BuildFrom([]job{{"a", 1, "x"}, {"b", 2, "x"}})
	q.Next()
	q.Cancel(1)
	q.Reset()
	if q.Len() != 0 || len(q.Cancelled()) != 0 {
		t.Fatal("Reset left something")
	}
	k := q.Add(job{"z", 0, "x"})
	if k != 0 || !slices.Equal(namesOfQueue(q), []string{"z"}) {
		t.Fatal("the queue does not go on from 0 after Reset")
	}
}

func BenchmarkQueueBuildFromAndTakeAll(b *testing.B) {
	jobs := make([]job, 1000)
	for i := range jobs {
		jobs[i] = job{priority: i % 5, kind: "x"}
	}
	q := table.NewPriorityQueue(func(j job) int { return j.priority })
	b.ReportAllocs()
	for b.Loop() {
		q.BuildFrom(jobs)
		for {
			if _, _, ok := q.Next(); !ok {
				break
			}
		}
	}
}
