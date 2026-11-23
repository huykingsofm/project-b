package table_test

import (
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/shared/table"
)

func (j job) Priority() int { return j.priority }

type job struct {
	name     string
	priority int
	kind     string
}

func newJobs(t testing.TB) *table.PriorityQueue[job, int] {
	t.Helper()
	q := &table.PriorityQueue[job, int]{}
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
	q.Next() // a
	q.Next() // b
	q.Next() // c
	names := func(seq func(func(int64, job) bool)) (out []string) {
		for _, j := range seq {
			out = append(out, j.name)
		}
		return out
	}
	if got := names(q.All()); !slices.Equal(got, []string{"a", "b", "c", "d"}) {
		t.Fatalf("All %v, want every element, the ones taken and cancelled included", got)
	}
	if got := names(q.Pending()); !slices.Equal(got, []string{"d"}) {
		t.Fatalf("Pending %v, want [d]", got)
	}
	r := newJobs(t)
	r.BuildFrom([]job{{"a", 1, "x"}, {"b", 2, "x"}, {"c", 3, "x"}})
	r.Next()
	if got := names(r.Pending()); !slices.Equal(got, []string{"b", "c"}) {
		t.Fatalf("Pending %v, want [b c]", got)
	}
}

func TestLookupFindsTheElementsThatWereTakenAsWellAsTheOthers(t *testing.T) {
	q := newJobs(t)
	q.BuildFrom([]job{{"a", 1, "item"}, {"b", 2, "item"}, {"c", 3, "body"}, {"d", 4, "item"}})
	q.Next() // a is taken
	q.Next() // b is taken
	var seen []string
	for _, j := range q.Lookup(kind("item")) {
		seen = append(seen, j.name)
	}
	if !slices.Equal(seen, []string{"a", "b", "d"}) {
		t.Fatalf("walked %v, want [a b d]: the ones taken are found too", seen)
	}
	if j, ok := q.Get(1); !ok || j.name != "b" {
		t.Fatal("an element that was taken is not there to be read")
	}
}

func TestResetTakesEverythingOutAndTheQueueGoesOn(t *testing.T) {
	q := newJobs(t)
	q.BuildFrom([]job{{"a", 1, "x"}, {"b", 2, "x"}})
	q.Next()
	q.Reset()
	if q.Len() != 0 {
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
	q := &table.PriorityQueue[job, int]{}
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
