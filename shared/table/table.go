// Package table is a stablemap that can also find its values by something other than the key:
// by one or two columns of the value, which the table keeps up to date as values are set and
// deleted. It is what a state uses to find an entity by its kind, or by its owner and then its
// kind, without walking all of them.
//
// A column is declared with Index or Index2, which give a Handle, and read with Lookup and the
// methods after it. A column is a function of the value: a value is indexed by what it is when it
// is set, so a value is only changed by setting it again (there is no way to change one in place,
// which is what lets the columns stay right). What is found is visited in a stable order, which
// is the same for the same sequence of calls, but is neither the order the values were set in nor
// the order of the columns.
//
// It is a library of its own: it imports only stablemap and stableset.
package table

import (
	"iter"

	"github.com/huykingsofm/project-b/shared/stablemap"
)

// Table is a stablemap of values by key, with the columns that were declared on it. The zero Table
// is empty and ready to use. It is not safe for concurrent use: what another goroutine reads is a
// Clone of it.
type Table[K comparable, V any] struct {
	data    stablemap.Map[K, V]
	columns []column[K, V]
}

// column is what the table asks of each of its columns, whatever the type of its key.
type column[K comparable, V any] interface {
	add(k K, v V)
	remove(k K, v V)
	clone() column[K, V]
}

// Handle names a column of one key, made by Index. It is a number, so it stays right in a Clone of
// the table, and it only means something to the table that made it, or to a clone of it.
type Handle[IK comparable] struct{ pos int }

// Handle2 names a column of two keys, made by Index2.
type Handle2[A, B comparable] struct{ pos int }

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
		for _, c := range t.columns {
			c.remove(k, old)
		}
	}
	t.data.Set(k, v)
	for _, c := range t.columns {
		c.add(k, v)
	}
}

// Delete takes the key out and reports whether it was there.
func (t *Table[K, V]) Delete(k K) bool {
	old, ok := t.data.Get(k)
	if !ok {
		return false
	}
	for _, c := range t.columns {
		c.remove(k, old)
	}
	return t.data.Delete(k)
}

// Clone returns a table that shares nothing with t, with the same columns, so a Handle of t is a
// Handle of the clone. A value is copied with its Clone when it has one.
func (t *Table[K, V]) Clone() Table[K, V] {
	c := Table[K, V]{data: t.data.Clone(), columns: make([]column[K, V], len(t.columns))}
	for i, col := range t.columns {
		c.columns[i] = col.clone()
	}
	return c
}

// Index declares a column of one key: keyOf says the key of a value, and false when the value is
// not in the column. The values that are in the table already are put in it.
func (t *Table[K, V]) Index[IK comparable](keyOf func(V) (IK, bool)) Handle[IK] {
	col := &column1[K, V, IK]{keyOf: keyOf}
	t.attach(col)
	return Handle[IK]{pos: len(t.columns) - 1}
}

// Index2 declares a column of two keys, found by the first alone or by both.
func (t *Table[K, V]) Index2[A, B comparable](keyOf func(V) (A, B, bool)) Handle2[A, B] {
	col := &column2[K, V, A, B]{keyOf: keyOf}
	t.attach(col)
	return Handle2[A, B]{pos: len(t.columns) - 1}
}

func (t *Table[K, V]) attach(col column[K, V]) {
	t.columns = append(t.columns, col)
	for k, v := range t.data.All() {
		col.add(k, v)
	}
}

// Lookup walks the values whose column has the key, in the stable order. A handle that is not a
// column of this table finds nothing.
func (t *Table[K, V]) Lookup[IK comparable](h Handle[IK], ik IK) iter.Seq2[K, V] {
	col, ok := t.columnAt[*column1[K, V, IK]](h.pos)
	if !ok {
		return empty[K, V]
	}
	return t.values(col.keys(ik))
}

// LookupAs is Lookup that gives each value as a T, and leaves out the values that are not one.
func (t *Table[K, V]) LookupAs[IK comparable, T any](h Handle[IK], ik IK) iter.Seq2[K, T] {
	return as[K, V, T](t.Lookup(h, ik))
}

// Lookup2First walks the values whose column has the first key, whatever the second.
func (t *Table[K, V]) Lookup2First[A, B comparable](h Handle2[A, B], a A) iter.Seq2[K, V] {
	col, ok := t.columnAt[*column2[K, V, A, B]](h.pos)
	if !ok {
		return empty[K, V]
	}
	return t.values(col.keysOfFirst(a))
}

// Lookup2 walks the values whose column has both keys.
func (t *Table[K, V]) Lookup2[A, B comparable](h Handle2[A, B], a A, b B) iter.Seq2[K, V] {
	col, ok := t.columnAt[*column2[K, V, A, B]](h.pos)
	if !ok {
		return empty[K, V]
	}
	return t.values(col.keysOf(a, b))
}

// Lookup2As is Lookup2 that gives each value as a T, and leaves out the values that are not one.
func (t *Table[K, V]) Lookup2As[A, B comparable, T any](h Handle2[A, B], a A, b B) iter.Seq2[K, T] {
	return as[K, V, T](t.Lookup2(h, a, b))
}

// Lookup2FirstAs is Lookup2First that gives each value as a T, and leaves out the values that are
// not one.
func (t *Table[K, V]) Lookup2FirstAs[A, B comparable, T any](h Handle2[A, B], a A) iter.Seq2[K, T] {
	return as[K, V, T](t.Lookup2First(h, a))
}

// values gives the value of each key that a column found. A column and the data agree, so a key
// the data does not have is not there.
func (t *Table[K, V]) values(keys iter.Seq[K]) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for k := range keys {
			v, ok := t.data.Get(k)
			if ok && !yield(k, v) {
				return
			}
		}
	}
}

// columnAt returns the column at a position when it is a C: a handle that is not of this table, or
// of another kind of column, finds none.
func (t *Table[K, V]) columnAt[C column[K, V]](pos int) (C, bool) {
	var zero C
	if pos < 0 || pos >= len(t.columns) {
		return zero, false
	}
	c, ok := t.columns[pos].(C)
	return c, ok
}

func empty[K comparable, V any](func(K, V) bool) {}

func as[K comparable, V, T any](seq iter.Seq2[K, V]) iter.Seq2[K, T] {
	return func(yield func(K, T) bool) {
		for k, v := range seq {
			if t, ok := any(v).(T); ok && !yield(k, t) {
				return
			}
		}
	}
}
