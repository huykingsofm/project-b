package stableset_test

import (
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/shared/stableset"
)

func TestASetKeepsAStableOrderAndADeleteMovesTheLastIntoItsPlace(t *testing.T) {
	var s stableset.Set[int]
	for _, k := range []int{1, 2, 3, 4} {
		s.Add(k)
	}
	s.Add(2) // already there: it stays
	if !s.Delete(2) || s.Delete(9) || s.Len() != 3 || s.Has(2) {
		t.Fatal("Delete does not follow what the set holds")
	}
	if got := slices.Collect(s.All()); !slices.Equal(got, []int{1, 4, 3}) {
		t.Fatalf("order %v, want [1 4 3]: the last element took the place of the one deleted", got)
	}
}

func TestAClonedSetSharesNothing(t *testing.T) {
	var s stableset.Set[int]
	s.Add(1)
	c := s.Clone()
	s.Add(2)
	c.Add(3)
	if slices.Collect(s.All())[1] != 2 || slices.Collect(c.All())[1] != 3 || s.Has(3) || c.Has(2) {
		t.Fatal("the clone and the set share memory")
	}
}
