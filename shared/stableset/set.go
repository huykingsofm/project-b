// Package stableset is a set whose elements are visited in a stable order, as the elements of a
// stablemap are: the same sequence of Add and Delete gives the same order, which is not the order
// the elements were added in once one was deleted.
package stableset

import (
	"iter"

	"github.com/huykingsofm/project-b/shared/stablemap"
)

// Set is a set of keys, made on a stablemap.Map. The zero Set is empty and ready to use. It is
// not safe for concurrent use.
type Set[K comparable] struct{ m stablemap.Map[K, struct{}] }

// Len returns how many elements the set holds.
func (s *Set[K]) Len() int { return s.m.Len() }

// Has reports whether the set holds the key.
func (s *Set[K]) Has(k K) bool { return s.m.Has(k) }

// Add puts a key in the set. A key that is there stays where it is.
func (s *Set[K]) Add(k K) {
	if !s.m.Has(k) {
		s.m.Set(k, struct{}{})
	}
}

// Delete takes a key out of the set and reports whether it was there. The last element moves into
// its place, so a delete costs O(1).
func (s *Set[K]) Delete(k K) bool { return s.m.Delete(k) }

// All walks the elements in the stable order.
func (s *Set[K]) All() iter.Seq[K] {
	return func(yield func(K) bool) {
		for _, k := range s.m.Keys() {
			if !yield(k) {
				return
			}
		}
	}
}

// Clone returns a set that shares nothing with s.
func (s *Set[K]) Clone() Set[K] { return Set[K]{m: s.m.Clone()} }
