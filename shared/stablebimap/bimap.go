// Package bimap holds a map that links the values of one type to the values of another, one to one,
// and can be read from either side. It imports nothing of the project.
package stablebimap

import (
	"iter"
	"maps"
	"slices"
)

// BiMap links each V1 to one V2 and each V2 to one V1. The two sides are the same in every way
// except which type is which.
//
// Every method that takes or gives one side has a twin for the other (GetByV1 and GetByV2,
// UnlinkByV1 and UnlinkByV2, AllV1 and AllV2).
//
// A link is one position in two slices, vals1 and vals2, and each side has an index from its value
// to that position. A link is stored once, so a Clone copies two slices and two small indexes. Link
// and Unlink change both sides at once, and there is no other way to change it, so the sides cannot
// disagree.
//
// The links are visited in the order they were made, except that removing one moves the last link
// into its place (as stablemap.Map does, so a removal costs O(1)). The order is a function of the
// sequence of Link and Unlink calls and of nothing else.
//
// The zero BiMap is empty and ready to use. It is not safe for concurrent use: the goroutine that
// owns it is the only one that touches it. What another goroutine reads is a Clone of it.
type BiMap[V1, V2 comparable] struct {
	index1 map[V1]int
	index2 map[V2]int
	vals1  []V1
	vals2  []V2
}

// Len returns how many links there are.
func (b *BiMap[V1, V2]) Len() int { return len(b.vals1) }

// Link links v1 to v2. The links are one to one, so a link that either of them already had is
// replaced: v1 leaves the V2 it had, and v2 leaves the V1 it had.
func (b *BiMap[V1, V2]) Link(v1 V1, v2 V2) {
	if b.index1 == nil {
		b.index1, b.index2 = map[V1]int{}, map[V2]int{}
	}
	if i, ok := b.index2[v2]; ok && b.vals1[i] != v1 {
		b.removeAt(i) // v2 belonged to another V1: it is taken from it
	}
	if i, ok := b.index1[v1]; ok {
		delete(b.index2, b.vals2[i])
		b.vals2[i] = v2
		b.index2[v2] = i
		return
	}
	i := len(b.vals1)
	b.vals1 = append(b.vals1, v1)
	b.vals2 = append(b.vals2, v2)
	b.index1[v1], b.index2[v2] = i, i
}

// UnlinkByV1 removes the link of v1. It reports whether v1 was linked.
func (b *BiMap[V1, V2]) UnlinkByV1(v1 V1) bool {
	i, ok := b.index1[v1]
	if !ok {
		return false
	}
	b.removeAt(i)
	return true
}

// UnlinkByV2 removes the link of v2. It reports whether v2 was linked.
func (b *BiMap[V1, V2]) UnlinkByV2(v2 V2) bool {
	i, ok := b.index2[v2]
	if !ok {
		return false
	}
	b.removeAt(i)
	return true
}

// removeAt removes the link at a position: the last link moves into its place.
func (b *BiMap[V1, V2]) removeAt(i int) {
	last := len(b.vals1) - 1
	delete(b.index1, b.vals1[i])
	delete(b.index2, b.vals2[i])
	if i != last {
		b.vals1[i], b.vals2[i] = b.vals1[last], b.vals2[last]
		b.index1[b.vals1[i]] = i
		b.index2[b.vals2[i]] = i
	}
	var zero1 V1
	var zero2 V2
	b.vals1[last], b.vals2[last] = zero1, zero2 // hold nothing that is no longer linked
	b.vals1 = b.vals1[:last]
	b.vals2 = b.vals2[:last]
}

// GetByV1 returns the V2 that v1 is linked to.
func (b *BiMap[V1, V2]) GetByV1(v1 V1) (V2, bool) {
	i, ok := b.index1[v1]
	if !ok {
		var zero V2
		return zero, false
	}
	return b.vals2[i], true
}

// GetByV2 returns the V1 that v2 is linked to.
func (b *BiMap[V1, V2]) GetByV2(v2 V2) (V1, bool) {
	i, ok := b.index2[v2]
	if !ok {
		var zero V1
		return zero, false
	}
	return b.vals1[i], true
}

// AllV1 visits the links in a fixed order, each as a V1 and the V2 it is linked to.
func (b *BiMap[V1, V2]) AllV1() iter.Seq2[V1, V2] {
	return func(yield func(V1, V2) bool) {
		for i := range b.vals1 {
			if !yield(b.vals1[i], b.vals2[i]) {
				return
			}
		}
	}
}

// AllV2 visits the same links in the same order, each as a V2 and the V1 it is linked to.
func (b *BiMap[V1, V2]) AllV2() iter.Seq2[V2, V1] {
	return func(yield func(V2, V1) bool) {
		for i := range b.vals2 {
			if !yield(b.vals2[i], b.vals1[i]) {
				return
			}
		}
	}
}

// Clone returns a copy that shares nothing with the map. The values are identifiers, comparable
// values that are never changed in place, so they are copied by assignment.
func (b *BiMap[V1, V2]) Clone() BiMap[V1, V2] {
	return BiMap[V1, V2]{
		index1: maps.Clone(b.index1), index2: maps.Clone(b.index2),
		vals1: slices.Clone(b.vals1), vals2: slices.Clone(b.vals2),
	}
}
