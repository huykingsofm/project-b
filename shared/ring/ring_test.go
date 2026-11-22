package ring_test

import (
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/shared/ring"
)

func TestTheOldestRowIsLetGoWhenTheRingIsFullAndTheOrderIsKept(t *testing.T) {
	r := ring.New[int](3)
	for i := 1; i <= 5; i++ {
		r.Add(i)
	}
	if got := r.Rows(); !slices.Equal(got, []int{3, 4, 5}) || r.Len() != 3 {
		t.Fatalf("rows %v, want [3 4 5]", got)
	}
	var seen []int
	for row := range r.All() {
		seen = append(seen, row)
		if len(seen) == 2 {
			break
		}
	}
	if !slices.Equal(seen, []int{3, 4}) {
		t.Fatalf("walked %v, want the oldest two", seen)
	}
}

func TestAClonedRingSharesNothingAndGoesOnFromWhereItWas(t *testing.T) {
	r := ring.New[int](3)
	for i := 1; i <= 4; i++ {
		r.Add(i)
	}
	c := r.Clone()
	r.Add(5)
	c.Add(9)
	if !slices.Equal(r.Rows(), []int{3, 4, 5}) || !slices.Equal(c.Rows(), []int{3, 4, 9}) {
		t.Fatalf("original %v, clone %v", r.Rows(), c.Rows())
	}
}

func TestARingOfNothingKeepsNothingAndAFewRowsAreAllKept(t *testing.T) {
	none := ring.New[int](0)
	none.Add(1)
	if none.Len() != 0 {
		t.Fatal("a ring with a limit of 0 kept a row")
	}
	few := ring.New[int](5)
	few.Add(1)
	few.Add(2)
	if !slices.Equal(few.Rows(), []int{1, 2}) {
		t.Fatalf("rows %v", few.Rows())
	}
}
