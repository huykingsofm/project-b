package bitset_test

import (
	"errors"
	"math/rand/v2"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/shared/bitset"
)

func mustNew(t testing.TB, k int) *bitset.Array {
	t.Helper()
	a, err := bitset.New(k)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestANewArrayHasKBitsAllClear(t *testing.T) {
	for _, k := range []int{0, 1, 63, 64, 65, 1000} {
		a := mustNew(t, k)
		if a.Len() != k || a.Count() != 0 {
			t.Fatalf("k=%d: Len %d, Count %d", k, a.Len(), a.Count())
		}
		for i := 0; i < k; i++ {
			if a.Get(i) {
				t.Fatalf("k=%d: bit %d is set", k, i)
			}
		}
	}
}

func TestANegativeLengthIsRefused(t *testing.T) {
	if a, err := bitset.New(-1); a != nil || !errors.Is(err, bitset.ErrNegativeLength) {
		t.Fatalf("got %v, %v", a, err)
	}
}

func TestSetAndClearChangeOnlyTheirBitAcrossWordEdges(t *testing.T) {
	a := mustNew(t, 130)
	for _, i := range []int{0, 1, 62, 63, 64, 65, 127, 128, 129} {
		if !a.Set(i) || !a.Get(i) || a.Count() != 1 {
			t.Fatalf("bit %d: Set did not set exactly it (Count %d)", i, a.Count())
		}
		if !a.Clear(i) || a.Get(i) || a.Count() != 0 {
			t.Fatalf("bit %d: Clear did not clear it (Count %d)", i, a.Count())
		}
	}
}

func TestABitOutsideTheArrayReadsClearAndCannotBeChanged(t *testing.T) {
	a := mustNew(t, 70)
	for _, i := range []int{-1, 70, 71, 127, 128, 1 << 40, -(1 << 40)} {
		if a.Get(i) || a.Set(i) || a.Clear(i) {
			t.Fatalf("bit %d outside the array was accepted", i)
		}
	}
	if a.Count() != 0 {
		t.Fatalf("Count %d after refused sets", a.Count())
	}
}

func TestItAgreesWithASliceOfBoolOnRandomOperations(t *testing.T) {
	const k = 1000
	a, want := mustNew(t, k), make([]bool, k)
	r := rand.New(rand.NewPCG(1, 2))
	for n := 0; n < 20000; n++ {
		i := r.IntN(k)
		if r.IntN(2) == 0 {
			a.Set(i)
			want[i] = true
		} else {
			a.Clear(i)
			want[i] = false
		}
	}
	count := 0
	for i, w := range want {
		if a.Get(i) != w {
			t.Fatalf("bit %d: got %v, want %v", i, a.Get(i), w)
		}
		if w {
			count++
		}
	}
	if a.Count() != count {
		t.Fatalf("Count %d, want %d", a.Count(), count)
	}
}
