// Package orderedmap is a map whose elements are visited in a fixed order: the order of an
// array, never the order of a Go map, so that two runs fed the same inputs visit the same
// elements in the same order.
//
// It is a library of its own that happens to be written here: it imports only the package clone
// and knows nothing of what it is used for.
package orderedmap

import (
	"iter"
	"maps"
	"slices"

	"github.com/huykingsofm/project-b/server/internal/shared/clone"
)

// Map is an ordered map made of three parts:
//
//   - index maps a key to its position, to reach one element fast;
//   - keys is the array of keys, to visit the elements in a fixed order;
//   - vals is the array of values, at the same positions as keys.
//
// The order of keys is the order the keys were added in, except that deleting a key moves
// the last key into its place (swap-remove, so a delete costs O(1)). The order is a
// function of the sequence of Set and Delete calls and of nothing else.
//
// The zero Map is empty and ready to use. A Map is not safe for concurrent use: the
// goroutine of the engine that owns it is the only one that touches it. What another engine
// reads is a Clone of it.
type Map[K comparable, V any] struct {
	index map[K]int
	keys  []K
	vals  []V
}

// Len returns how many elements the map holds.
func (m *Map[K, V]) Len() int { return len(m.keys) }

// Has reports whether the key is in the map.
func (m *Map[K, V]) Has(k K) bool {
	_, ok := m.index[k]
	return ok
}

// Get returns the value of the key, and whether the key is in the map.
func (m *Map[K, V]) Get(k K) (V, bool) {
	i, ok := m.index[k]
	if !ok {
		var zero V
		return zero, false
	}
	return m.vals[i], true
}

// Ptr returns a pointer to the value of the key, or nil. The pointer is valid until the
// next Set of a new key or Delete, and lets the state change a value in place.
func (m *Map[K, V]) Ptr(k K) *V {
	i, ok := m.index[k]
	if !ok {
		return nil
	}
	return &m.vals[i]
}

// Set stores the value under the key. A new key goes to the end of the order.
func (m *Map[K, V]) Set(k K, v V) {
	if i, ok := m.index[k]; ok {
		m.vals[i] = v
		return
	}
	if m.index == nil {
		m.index = make(map[K]int)
	}
	m.index[k] = len(m.keys)
	m.keys = append(m.keys, k)
	m.vals = append(m.vals, v)
}

// Delete removes the key, and reports whether it was there. The last key takes its place
// in the order. Do not call it while visiting the map.
func (m *Map[K, V]) Delete(k K) bool {
	i, ok := m.index[k]
	if !ok {
		return false
	}
	last := len(m.keys) - 1
	if i != last {
		m.keys[i] = m.keys[last]
		m.vals[i] = m.vals[last]
		m.index[m.keys[i]] = i
	}
	var zk K
	var zv V
	m.keys[last], m.vals[last] = zk, zv
	m.keys, m.vals = m.keys[:last], m.vals[:last]
	delete(m.index, k)
	return true
}

// Keys returns the keys in order. The slice is a view: do not change it.
func (m *Map[K, V]) Keys() []K { return m.keys }

// At returns the key and the value at a position of the order.
func (m *Map[K, V]) At(i int) (K, V) { return m.keys[i], m.vals[i] }

// All visits the elements in order. Do not Set a new key or Delete while it runs.
func (m *Map[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for i, k := range m.keys {
			if !yield(k, m.vals[i]) {
				return
			}
		}
	}
}

// Clone returns a copy that shares nothing with the map. The values are copied with their Clone
// method when the type of the values has one (see package clone), and by assignment otherwise, so
// the type of the values must be a value (no pointer, slice, map, channel or function in it) or
// have Clone. The keys are copied by assignment: a key is an identifier, a value that is never
// changed in place.
func (m *Map[K, V]) Clone() Map[K, V] {
	return Map[K, V]{
		index: maps.Clone(m.index), keys: slices.Clone(m.keys), vals: clone.Slice(m.vals),
	}
}

// CloneFunc is Clone with a function that copies each value, for a type of values that has no Clone
// method of its own.
func (m *Map[K, V]) CloneFunc(copyValue func(V) V) Map[K, V] {
	c := Map[K, V]{
		index: maps.Clone(m.index), keys: slices.Clone(m.keys), vals: make([]V, len(m.vals)),
	}
	for i, v := range m.vals {
		c.vals[i] = copyValue(v)
	}
	return c
}
