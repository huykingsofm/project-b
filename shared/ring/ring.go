// Package ring holds the last rows of a log that only grows at the end: a fixed number of rows,
// and the oldest is let go when a new one does not fit. It is what a chat is, a list that is
// added to and read in order, with no key and nothing to find a row by.
package ring

import (
	"iter"

	"github.com/huykingsofm/project-b/shared/clone"
)

// Ring is a log of at most a limit of rows, oldest first. Make one with New: the zero value has a
// limit of 0 and keeps nothing.
type Ring[T any] struct {
	buf   []T // the rows, in a circle: the oldest is at start
	start int
	limit int
}

// New makes an empty ring that keeps the last limit rows.
func New[T any](limit int) Ring[T] { return Ring[T]{limit: max(limit, 0)} }

// Len returns how many rows the ring holds.
func (r *Ring[T]) Len() int { return len(r.buf) }

// Add puts a row at the end. When the ring is full the oldest row is let go.
func (r *Ring[T]) Add(row T) {
	switch {
	case r.limit == 0:
	case len(r.buf) < r.limit:
		r.buf = append(r.buf, row)
	default:
		r.buf[r.start] = row
		r.start = (r.start + 1) % r.limit
	}
}

// All walks the rows, oldest first.
func (r *Ring[T]) All() iter.Seq[T] {
	return func(yield func(T) bool) {
		for i := range r.buf {
			if !yield(r.buf[(r.start+i)%len(r.buf)]) {
				return
			}
		}
	}
}

// Rows returns the rows, oldest first, as a slice of their own.
func (r *Ring[T]) Rows() []T {
	out := make([]T, 0, len(r.buf))
	for row := range r.All() {
		out = append(out, row)
	}
	return clone.Slice(out)
}

// Clone returns a ring that shares nothing with r: a row is copied with its Clone when it has one.
func (r *Ring[T]) Clone() Ring[T] {
	return Ring[T]{buf: clone.Slice(r.Rows()), limit: r.limit}
}
