// Package table is a list of rows kept in memory, that rows are found in by the value of a
// column, and that remembers which rows changed.
//
// It is a library of its own that happens to be written here: it imports only the package clone
// and knows nothing of what it is used for.
package table

import (
	"maps"
	"slices"

	"github.com/huykingsofm/project-b/server/internal/shared/clone"
)

// Row is a type that a Table can hold. A row says by which columns it is found: Index returns,
// for each column, the value of the row in it. The values must be comparable (a number, a
// string, a struct of those), since they are used as keys of a Go map.
type Row interface {
	Index() map[string]any
}

// Table holds rows in the order they were added, with:
//
//   - rows: the rows themselves, at fixed positions;
//   - dirty: the positions of the rows that were added or set since the dirty record was
//     cleared, so that only what changed is written to the store;
//   - indexBy: for each column and each value, the positions of the rows that have that value
//     in that column, made from the Index of each row. FindBy reads it.
//
// The Go maps inside are only looked up, and are never visited in an order that matters: a
// position list is always in ascending order, so two runs fed the same inputs find the same
// rows in the same order.
//
// The zero Table is empty and ready to use. A Table is not safe for concurrent use by itself;
// what other goroutines read is a Clone of it, which they may read freely, FindBy included.
type Table[T Row] struct {
	rows    []T
	dirty   map[int]bool
	indexBy map[string]map[any][]int
}

// Build makes a table of the rows, and indexes them. The rows are not dirty.
func Build[T Row](rows []T) *Table[T] {
	t := &Table[T]{rows: slices.Clone(rows)}
	t.reindex()
	return t
}

// Len returns how many rows the table holds.
func (t *Table[T]) Len() int { return len(t.rows) }

// Get returns the row at a position.
func (t *Table[T]) Get(i int) (T, bool) {
	if i < 0 || i >= len(t.rows) {
		var zero T
		return zero, false
	}
	return t.rows[i], true
}

// Rows returns a copy of the rows, in order.
func (t *Table[T]) Rows() []T { return clone.Slice(t.rows) }

// Add appends a row, marks it dirty, and returns its position.
func (t *Table[T]) Add(row T) int {
	i := len(t.rows)
	t.rows = append(t.rows, row)
	t.markDirty(i)
	t.insert(i, row)
	return i
}

// Set replaces the row at a position. It does nothing if there is no such row.
func (t *Table[T]) Set(i int, row T) {
	if i < 0 || i >= len(t.rows) {
		return
	}
	t.remove(i, t.rows[i])
	t.rows[i] = row
	t.markDirty(i)
	t.insert(i, row)
}

// Delete removes the row at a position. The rows after it move up by one, so the positions
// of the dirty rows and of the indexes are made again: it costs as much as the table is long.
func (t *Table[T]) Delete(i int) {
	if i < 0 || i >= len(t.rows) {
		return
	}
	t.dropFront(i, i+1)
}

// Trim deletes the oldest rows until at most keep rows are left.
func (t *Table[T]) Trim(keep int) {
	if n := len(t.rows) - keep; n > 0 {
		t.dropFront(0, n)
	}
}

// dropFront removes rows[from:to] and fixes what holds positions.
func (t *Table[T]) dropFront(from, to int) {
	t.rows = slices.Delete(t.rows, from, to)
	if len(t.dirty) > 0 {
		next := make(map[int]bool, len(t.dirty))
		for i := range t.dirty {
			switch {
			case i < from:
				next[i] = true
			case i >= to:
				next[i-(to-from)] = true
			}
		}
		t.dirty = next
	}
	t.reindex()
}

// FindBy returns the rows whose value in the column is value, in the order they are in the
// table. The value must be comparable.
func (t *Table[T]) FindBy(column string, value any) []T {
	ids := t.indexBy[column][value]
	out := make([]T, 0, len(ids))
	for _, i := range ids {
		out = append(out, t.rows[i])
	}
	return out
}

// Dirty returns the positions of the rows that were added or set since ClearDirty, in
// ascending order.
func (t *Table[T]) Dirty() []int { return slices.Sorted(maps.Keys(t.dirty)) }

// ClearDirty forgets which rows changed.
func (t *Table[T]) ClearDirty() { t.dirty = nil }

// Clone returns a copy that shares nothing with the table: the rows are copied with their Clone method
// when the type of the rows has one (see package clone), and by assignment otherwise, so the type of the
// rows must be a value or have Clone. The indexes are copied too, so the copy can be searched at once. It
// has no dirty rows.
func (t *Table[T]) Clone() Table[T] {
	c := Table[T]{rows: clone.Slice(t.rows)}
	if t.indexBy != nil {
		c.indexBy = make(map[string]map[any][]int, len(t.indexBy))
		for column, byValue := range t.indexBy {
			m := make(map[any][]int, len(byValue))
			for value, ids := range byValue {
				m[value] = slices.Clone(ids)
			}
			c.indexBy[column] = m
		}
	}
	return c
}

func (t *Table[T]) markDirty(i int) {
	if t.dirty == nil {
		t.dirty = make(map[int]bool)
	}
	t.dirty[i] = true
}

// reindex makes all the indexes again from the rows.
func (t *Table[T]) reindex() {
	t.indexBy = nil
	for i, row := range t.rows {
		t.insert(i, row)
	}
}

// insert puts the position of a row under each of its values, keeping each list ascending.
func (t *Table[T]) insert(i int, row T) {
	for column, value := range row.Index() {
		if t.indexBy == nil {
			t.indexBy = make(map[string]map[any][]int)
		}
		byValue := t.indexBy[column]
		if byValue == nil {
			byValue = make(map[any][]int)
			t.indexBy[column] = byValue
		}
		ids := byValue[value]
		at, _ := slices.BinarySearch(ids, i)
		byValue[value] = slices.Insert(ids, at, i)
	}
}

// remove takes the position of a row out of each of its values.
func (t *Table[T]) remove(i int, row T) {
	for column, value := range row.Index() {
		byValue := t.indexBy[column]
		ids := byValue[value]
		if at, found := slices.BinarySearch(ids, i); found {
			ids = slices.Delete(ids, at, at+1)
		}
		if len(ids) == 0 {
			delete(byValue, value)
			continue
		}
		byValue[value] = ids
	}
}
