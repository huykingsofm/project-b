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
		for i := range k {
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
	for range 20000 {
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

func TestAnyInRangeAgreesWithLookingAtEveryBitOfTheRange(t *testing.T) {
	const k = 300
	r := rand.New(rand.NewPCG(5, 6))
	for range 200 {
		a, bools := mustNew(t, k), make([]bool, k)
		for i := 0; i < r.IntN(6); i++ { // few bits, so that ranges without any are common
			j := r.IntN(k)
			a.Set(j)
			bools[j] = true
		}
		for range 400 {
			// starts and ends outside the array, and lengths that are not positive
			lo, length := r.IntN(k+80)-40, r.IntN(200)-5
			want := false
			for i := lo; i < lo+length; i++ {
				if i >= 0 && i < k && bools[i] {
					want = true
				}
			}
			if got := a.AnyInRange(lo, length); got != want {
				t.Fatalf("AnyInRange(%d, %d) = %v, want %v", lo, length, got, want)
			}
		}
	}
}

func TestAnyInRangeAtTheEdgesOfWords(t *testing.T) {
	a := mustNew(t, 192)
	a.Set(63)
	for _, c := range []struct {
		lo, n int
		want  bool
	}{{0, 63, false}, {0, 64, true}, {63, 1, true}, {64, 128, false}, {62, 2, true}, {
		63, 1 << 62, true,
	}, {-1 << 62, 1 << 62, false}, {1 << 62, 1 << 62, false}} {
		if got := a.AnyInRange(c.lo, c.n); got != c.want {
			t.Errorf("AnyInRange(%d, %d) = %v, want %v", c.lo, c.n, got, c.want)
		}
	}
}
