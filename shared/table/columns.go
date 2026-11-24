package table

import (
	"errors"
	"fmt"
	"iter"
	"reflect"
)

// ErrColumnDeclared is the error of a column whose key type is already the key of a column of the
// table: a key type is of one column, and what Lookup finds is by the type of its key.
var ErrColumnDeclared = errors.New("table: a column of that key type is declared already")

// columns is the columns of a table or a queue, and how they are declared and found. They are
// found by the type of their key.
type columns[K comparable, V any] struct{ list []column[K, V] }

func (cs *columns[K, V]) add(k K, v V) {
	for _, c := range cs.list {
		c.add(k, v)
	}
}

func (cs *columns[K, V]) remove(k K, v V) {
	for _, c := range cs.list {
		c.remove(k, v)
	}
}

func (cs *columns[K, V]) change(k K, old, v V) {
	for _, c := range cs.list {
		c.change(k, old, v)
	}
}

func (cs *columns[K, V]) reset() {
	for _, c := range cs.list {
		c.reset()
	}
}

func (cs *columns[K, V]) clone() columns[K, V] {
	out := columns[K, V]{list: make([]column[K, V], len(cs.list))}
	for i, c := range cs.list {
		out.list[i] = c.clone()
	}
	return out
}

// declare adds a column of one key, and puts in it the values that are there already.
func (cs *columns[K, V]) declare1[IK comparable](
	keyOf func(V) (IK, bool), existing iter.Seq2[K, V],
) error {
	if _, ok := columnAt[K, V, *column1[K, V, IK]](cs); ok {
		return fmt.Errorf("%w: %s", ErrColumnDeclared, reflect.TypeFor[IK]())
	}
	cs.attach(&column1[K, V, IK]{keyOf: keyOf}, existing)
	return nil
}

// declare2 adds a column of two keys, and puts in it the values that are there already.
func (cs *columns[K, V]) declare2[A, B comparable](
	keyOf func(V) (A, B, bool), existing iter.Seq2[K, V],
) error {
	for _, c := range cs.list {
		if _, ok := c.(firstKeyed[K, A]); ok {
			return fmt.Errorf(
				"%w: a column of two keys that starts with %s",
				ErrColumnDeclared, reflect.TypeFor[A](),
			)
		}
	}
	cs.attach(&column2[K, V, A, B]{keyOf: keyOf}, existing)
	return nil
}

func (cs *columns[K, V]) attach(col column[K, V], existing iter.Seq2[K, V]) {
	cs.list = append(cs.list, col)
	for k, v := range existing {
		col.add(k, v)
	}
}

// keys1 is a view of the keys of the values whose column of the key type IK has the key.
func (cs *columns[K, V]) keys1[IK comparable](ik IK) []K {
	col, ok := columnAt[K, V, *column1[K, V, IK]](cs)
	if !ok {
		return nil
	}
	return col.keys(ik)
}

// keys2 is a view of the keys of the values whose column of the key types (A, B) has both keys.
func (cs *columns[K, V]) keys2[A, B comparable](a A, b B) []K {
	col, ok := columnAt[K, V, *column2[K, V, A, B]](cs)
	if !ok {
		return nil
	}
	return col.keysOf(a, b)
}

// keys2First is a view of the keys of the values whose column of two keys that starts with A has
// the first key.
func (cs *columns[K, V]) keys2First[A comparable](a A) []K {
	for _, c := range cs.list {
		if f, ok := c.(firstKeyed[K, A]); ok {
			return f.keysOfFirst(a)
		}
	}
	return nil
}

// firstKeyed is what a column of two keys is for a lookup by its first key, whatever its second.
type firstKeyed[K comparable, A comparable] interface{ keysOfFirst(a A) []K }

// columnAt returns the column of the type C, that is, the column of a key type.
func columnAt[K comparable, V any, C column[K, V]](cs *columns[K, V]) (C, bool) {
	for _, c := range cs.list {
		if typed, ok := c.(C); ok {
			return typed, true
		}
	}
	var zero C
	return zero, false
}
