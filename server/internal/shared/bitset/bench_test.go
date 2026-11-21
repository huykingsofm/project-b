package bitset_test

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/shared/bitset"
)

// sizes are powers of two so that a sequential index wraps with a mask: 64 Ki bits fit the L1 cache
// either way, 1 Mi bits fit the L2 cache as bits and not as bytes, 16 Mi bits (a map of 4096 by 4096
// tiles) fit no cache as bytes.
var sizes = []int{1 << 16, 1 << 20, 1 << 24}

// indexes is a table of random indexes below size, read in a loop so that the cost of picking one is
// the same for both kinds.
func indexes(size int) []int {
	r := rand.New(rand.NewPCG(1, 2))
	idx := make([]int, 1<<12)
	for i := range idx {
		idx[i] = r.IntN(size)
	}
	return idx
}

// filled makes both kinds with the same pattern: about one bit in four set.
func filled(b *testing.B, size int) (*bitset.Array, []bool) {
	a, err := bitset.New(size)
	if err != nil {
		b.Fatal(err)
	}
	bools := make([]bool, size)
	r := rand.New(rand.NewPCG(3, 4))
	for i := 0; i < size; i += 1 + r.IntN(7) {
		a.Set(i)
		bools[i] = true
	}
	return a, bools
}

var sink int

func BenchmarkGetSequential(b *testing.B) {
	for _, size := range sizes {
		a, bools := filled(b, size)
		mask := size - 1
		b.Run(fmt.Sprintf("bits=%d/bitset", size), func(b *testing.B) {
			n := 0
			for i := 0; i < b.N; i++ {
				if a.Get(i & mask) {
					n++
				}
			}
			sink = n
		})
		b.Run(fmt.Sprintf("bits=%d/bool", size), func(b *testing.B) {
			n := 0
			for i := 0; i < b.N; i++ {
				if bools[i&mask] {
					n++
				}
			}
			sink = n
		})
	}
}

func BenchmarkGetRandom(b *testing.B) {
	for _, size := range sizes {
		a, bools := filled(b, size)
		idx := indexes(size)
		mask := len(idx) - 1
		b.Run(fmt.Sprintf("bits=%d/bitset", size), func(b *testing.B) {
			n := 0
			for i := 0; i < b.N; i++ {
				if a.Get(idx[i&mask]) {
					n++
				}
			}
			sink = n
		})
		b.Run(fmt.Sprintf("bits=%d/bool", size), func(b *testing.B) {
			n := 0
			for i := 0; i < b.N; i++ {
				if bools[idx[i&mask]] {
					n++
				}
			}
			sink = n
		})
	}
}

func BenchmarkSetRandom(b *testing.B) {
	for _, size := range sizes {
		a, bools := filled(b, size)
		idx := indexes(size)
		mask := len(idx) - 1
		b.Run(fmt.Sprintf("bits=%d/bitset", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				a.Set(idx[i&mask])
			}
		})
		b.Run(fmt.Sprintf("bits=%d/bool", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				bools[idx[i&mask]] = true
			}
		})
	}
}
