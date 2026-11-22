// Package table is a stablemap that can also find its values by something other than the key: by
// one or two columns of the value, which the table keeps up to date as values are set and
// deleted. It is what a state uses to find an entity by its kind, or by its owner and then its
// kind, without walking all of them. A PriorityQueue is the same columns on a list that is taken
// in order.
//
// A column is declared with Index or Index2 and found by the type of its key: Lookup(kind) reads
// the column whose key is a KindID, so there is no handle to keep, and a key type is of one
// column. A column is a function of the value: a value is indexed by what it is when it is set, so
// a value is only changed by setting it again (there is no way to change one in place, which is
// what lets the columns stay right). What is found is visited in a stable order, which is the same
// for the same sequence of calls, but is neither the order the values were set in nor the order of
// the columns. Deleting a value costs the size of the buckets it is in.
//
// It is a library of its own: it imports only stablemap.
package table

import (
	"iter"

	"github.com/huykingsofm/project-b/shared/stablemap"
)

// Table is a stablemap of values by key, with the columns that were declared on it. The zero Table
// is empty and ready to use. It is not safe for concurrent use: what another goroutine reads is a
// Clone of it.
type Table[K comparable, V any] struct {
	data stablemap.Map[K, V]
	cols columns[K, V]
}

// Len returns how many values the table holds.
func (t *Table[K, V]) Len() int { return t.data.Len() }

// Has reports whether the table holds the key.
func (t *Table[K, V]) Has(k K) bool { return t.data.Has(k) }

// Get returns the value under the key.
func (t *Table[K, V]) Get(k K) (V, bool) { return t.data.Get(k) }

// Keys returns the keys in the stable order. The slice is a view: do not change it.
func (t *Table[K, V]) Keys() []K { return t.data.Keys() }

// All walks the values in the stable order.
func (t *Table[K, V]) All() iter.Seq2[K, V] { return t.data.All() }

// Set puts a value under the key, in place of the one that was there, and brings the columns to
// what the new value says.
func (t *Table[K, V]) Set(k K, v V) {
	if old, ok := t.data.Get(k); ok {
		t.cols.remove(k, old)
	}
	t.data.Set(k, v)
	t.cols.add(k, v)
}

// Delete takes the key out and reports whether it was there.
func (t *Table[K, V]) Delete(k K) bool {
	old, ok := t.data.Get(k)
	if !ok {
		return false
	}
	t.cols.remove(k, old)
	return t.data.Delete(k)
}

// Clone returns a table that shares nothing with t, with the same columns. A value is copied with
// its Clone when it has one.
func (t *Table[K, V]) Clone() Table[K, V] {
	return Table[K, V]{data: t.data.Clone(), cols: t.cols.clone()}
}

// Reset takes every value out and keeps the memory and the columns, so a table that is filled and
// emptied again and again does not allocate each time.
func (t *Table[K, V]) Reset() {
	t.data.Clear()
	t.cols.reset()
}

// Index declares a column of one key: keyOf says the key of a value, and false when the value is
// not in the column. The column is the one of its key type IK, which Lookup finds it by, so a
// second column with the same key type is an error: give each a type of its own (a defined type,
// as a kind or an owner is). The values that are in the table already are put in the column.
func (t *Table[K, V]) Index[IK comparable](keyOf func(V) (IK, bool)) error {
	return t.cols.declare1(keyOf, t.data.All())
}

// Index2 declares a column of two keys, found by the first alone or by both. The column is the one
// of the key types (A, B), and no other column of two keys has the first key type A.
func (t *Table[K, V]) Index2[A, B comparable](keyOf func(V) (A, B, bool)) error {
	return t.cols.declare2(keyOf, t.data.All())
}

// Lookup walks the values whose column of the key type IK has the key, in the stable order. A key
// type that is the key of no column finds nothing.
func (t *Table[K, V]) Lookup[IK comparable](ik IK) iter.Seq2[K, V] {
	return t.values(t.cols.keys1(ik))
}

// LookupAs is Lookup that gives each value as a T, and leaves out the values that are not one.
func (t *Table[K, V]) LookupAs[IK comparable, T any](ik IK) iter.Seq2[K, T] {
	return as[K, V, T](t.Lookup(ik))
}

// Lookup2 walks the values whose column of the key types (A, B) has both keys.
func (t *Table[K, V]) Lookup2[A, B comparable](a A, b B) iter.Seq2[K, V] {
	return t.values(t.cols.keys2(a, b))
}

// Lookup2As is Lookup2 that gives each value as a T, and leaves out the values that are not one.
func (t *Table[K, V]) Lookup2As[A, B comparable, T any](a A, b B) iter.Seq2[K, T] {
	return as[K, V, T](t.Lookup2(a, b))
}

// Lookup2First walks the values whose column of two keys that starts with the key type A has the
// first key, whatever the second.
func (t *Table[K, V]) Lookup2First[A comparable](a A) iter.Seq2[K, V] {
	return t.values(t.cols.keys2First(a))
}

// Lookup2FirstAs is Lookup2First that gives each value as a T, and leaves out the values that are
// not one.
func (t *Table[K, V]) Lookup2FirstAs[A comparable, T any](a A) iter.Seq2[K, T] {
	return as[K, V, T](t.Lookup2First(a))
}

// values gives the value of each key that a column found. The columns and the data agree, so a
// key the data does not have is not there.
func (t *Table[K, V]) values(keys []K) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for _, k := range keys {
			v, ok := t.data.Get(k)
			if ok && !yield(k, v) {
				return
			}
		}
	}
}

func as[K comparable, V, T any](seq iter.Seq2[K, V]) iter.Seq2[K, T] {
	return func(yield func(K, T) bool) {
		for k, v := range seq {
			if t, ok := any(v).(T); ok && !yield(k, t) {
				return
			}
		}
	}
}
