package table

import (
	"cmp"
	"iter"
	"slices"
	"sort"
)

// slot is where an element stands in the order of a queue.
type slot[P cmp.Ordered] struct {
	key int64
	p   P
}

// PriorityQueue is a list of elements that are taken one by one, the one with the smallest priority
// first, and the ones of the same priority in the order they came. It remembers every element it
// held in this round until Reset, the ones that were taken and the ones still to be taken, and
// walks them in that order. It has the columns of a Table, which find every element it holds, taken
// or not: what the queue says of an element is only where it stands, and what else is true of it
// (that it was cancelled, for one) is for the element to say.
//
// An element that is added while others are being taken goes after the one taken last: it is
// ordered among the elements still to be taken and never before one that was taken already, so one
// that is smaller than all of them is the next to be taken. The elements are added one by one with
// Add, or all at once with BuildFrom, which orders them once instead of at each Add.
//
// The key of an element is a number that the queue gives it, in the order they were added, from 0
// after each Reset. The zero PriorityQueue is not ready: make one with NewPriorityQueue. It is not
// safe for concurrent use. Do not Add while a Pending or a Lookup is being walked.
type PriorityQueue[V any, P cmp.Ordered] struct {
	vals     []V // by key
	cols     columns[int64, V]
	priority func(V) P
	order    []slot[P] // every element, from the first to the last it will be taken
	next     int       // the position in order of the element that is taken next
}

// NewPriorityQueue makes an empty queue. priorityOf says the priority of an element: the smaller
// is taken first.
func NewPriorityQueue[V any, P cmp.Ordered](priorityOf func(V) P) *PriorityQueue[V, P] {
	return &PriorityQueue[V, P]{priority: priorityOf}
}

// Len returns how many elements the queue holds, the ones taken included.
func (q *PriorityQueue[V, P]) Len() int { return len(q.vals) }

// Get returns the element with the key, taken or not.
func (q *PriorityQueue[V, P]) Get(key int64) (V, bool) {
	if key < 0 || key >= int64(len(q.vals)) {
		var zero V
		return zero, false
	}
	return q.vals[key], true
}

// Reset takes every element out and keeps the memory and the columns.
func (q *PriorityQueue[V, P]) Reset() {
	clear(q.vals)
	q.vals = q.vals[:0]
	q.cols.reset()
	clear(q.order)
	q.order, q.next = q.order[:0], 0
}

// add stores an element and gives it its key, and does not put it in the order.
func (q *PriorityQueue[V, P]) add(v V) slot[P] {
	key := int64(len(q.vals))
	q.vals = append(q.vals, v)
	q.cols.add(key, v)
	return slot[P]{key: key, p: q.priority(v)}
}

// Add puts an element in the queue, in its place among the elements still to be taken, and returns
// its key.
func (q *PriorityQueue[V, P]) Add(v V) int64 {
	s := q.add(v)
	rest := q.order[q.next:]
	at := q.next + sort.Search(len(rest), func(i int) bool { return rest[i].p > s.p })
	q.order = slices.Insert(q.order, at, s)
	return s.key
}

// BuildFrom makes the queue hold the elements, in the order they are given for the same priority,
// and takes out what it held. It is what Reset and an Add of each element make, with the order done
// once.
func (q *PriorityQueue[V, P]) BuildFrom(vs []V) {
	q.Reset()
	for _, v := range vs {
		q.order = append(q.order, q.add(v))
	}
	// the keys rise with the position, so a stable sort by priority is the order of (priority, key)
	slices.SortStableFunc(q.order, func(a, b slot[P]) int { return cmp.Compare(a.p, b.p) })
}

// Next takes the next element, the one with the smallest priority among those not taken, and of
// those the first that came. It returns false when there is none.
func (q *PriorityQueue[V, P]) Next() (int64, V, bool) {
	if q.next >= len(q.order) {
		var zero V
		return 0, zero, false
	}
	key := q.order[q.next].key
	q.next++
	return key, q.vals[key], true
}

// All walks every element from the first of the order, the ones that were taken included.
func (q *PriorityQueue[V, P]) All() iter.Seq2[int64, V] {
	return func(yield func(int64, V) bool) {
		for _, s := range q.order {
			if !yield(s.key, q.vals[s.key]) {
				return
			}
		}
	}
}

// Pending walks the elements not taken yet, from the next one, in the order Next gives them.
func (q *PriorityQueue[V, P]) Pending() iter.Seq2[int64, V] {
	return func(yield func(int64, V) bool) {
		for i := q.next; i < len(q.order); i++ {
			key := q.order[i].key
			if !yield(key, q.vals[key]) {
				return
			}
		}
	}
}

// Index declares a column of one key, as Table.Index does. Lookup finds the elements that were
// taken as well as the others.
func (q *PriorityQueue[V, P]) Index[IK comparable](keyOf func(V) (IK, bool)) error {
	return q.cols.declare1(keyOf, q.All())
}

// Index2 declares a column of two keys, as Table.Index2 does.
func (q *PriorityQueue[V, P]) Index2[A, B comparable](keyOf func(V) (A, B, bool)) error {
	return q.cols.declare2(keyOf, q.All())
}

// each gives the elements of the keys. It is a function of its own, not a closure, so that a
// Lookup is small enough to be put where it is walked and makes no allocation.
func (q *PriorityQueue[V, P]) each(keys []int64, yield func(int64, V) bool) {
	for _, k := range keys {
		if !yield(k, q.vals[k]) {
			return
		}
	}
}

// Lookup walks the elements whose column of the key type IK has the key, taken or not, in the
// stable order of the column (the order they were added in, since the queue never takes one out of
// its columns before Reset).
func (q *PriorityQueue[V, P]) Lookup[IK comparable](ik IK) iter.Seq2[int64, V] {
	return func(yield func(int64, V) bool) { q.each(q.cols.keys1(ik), yield) }
}

// LookupAs is Lookup that gives each element as a T, and leaves out the ones that are not one.
func (q *PriorityQueue[V, P]) LookupAs[IK comparable, T any](ik IK) iter.Seq2[int64, T] {
	return as[int64, V, T](q.Lookup(ik))
}

// Lookup2 walks the elements whose column of the key types (A, B) has both keys.
func (q *PriorityQueue[V, P]) Lookup2[A, B comparable](a A, b B) iter.Seq2[int64, V] {
	return func(yield func(int64, V) bool) { q.each(q.cols.keys2(a, b), yield) }
}

// Lookup2As is Lookup2 that gives each element as a T, and leaves out the ones that are not one.
func (q *PriorityQueue[V, P]) Lookup2As[A, B comparable, T any](a A, b B) iter.Seq2[int64, T] {
	return as[int64, V, T](q.Lookup2(a, b))
}

// Lookup2First walks the elements whose column of two keys that starts with the
// key type A has the first key.
func (q *PriorityQueue[V, P]) Lookup2First[A comparable](a A) iter.Seq2[int64, V] {
	return func(yield func(int64, V) bool) { q.each(q.cols.keys2First(a), yield) }
}

// Lookup2FirstAs is Lookup2First that gives each element as a T, and leaves out the ones that are
// not one.
func (q *PriorityQueue[V, P]) Lookup2FirstAs[A comparable, T any](a A) iter.Seq2[int64, T] {
	return as[int64, V, T](q.Lookup2First(a))
}
