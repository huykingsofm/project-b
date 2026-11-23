// Package ids gives out the IDs of things that come and go while a game runs, and keeps an ID that
// was given back in a grave for a number of steps before it can be given again, so that what still
// points at the old one has had the time to let go.
//
// An allocator never gives 0, gives the IDs that were given back before the ones it has never
// given, and gives back first the one that has been free longest. What it holds is plain data, a
// number and two lists in an order that is fixed, so that it can be written to a checkpoint and
// worked into the hash of a state, and a replay of the same calls ends with the same allocator.
// It is not safe for concurrent use: the state that owns it is changed by one goroutine.
package ids

import (
	"fmt"
	"iter"
	"slices"

	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ID is what an allocator gives out: a number.
type ID interface{ ~uint32 | ~uint64 }

// Status says what an ID is to an allocator.
type Status uint8

const (
	// NeverGiven is an ID that the allocator never gave, and 0, which it never gives.
	NeverGiven Status = iota + 1
	// Live is an ID that was given and not given back.
	Live
	// InGrave is an ID that was given back and has not waited the steps yet.
	InGrave
	// Free is an ID that was given back and has waited: it is given again before any new one.
	Free
)

// String is the name of the status.
func (s Status) String() string {
	switch s {
	case NeverGiven:
		return "never given"
	case Live:
		return "live"
	case InGrave:
		return "in the grave"
	case Free:
		return "free"
	}
	return "unknown"
}

// resting is an ID in the grave, and the step at which it leaves.
type resting[T ID] struct {
	id    T
	until uint64
}

// Allocator gives out IDs from 1 to a limit. Make one with New: the zero value has a limit of 0 and
// gives nothing.
type Allocator[T ID] struct {
	steps uint64 // how many steps an ID waits in the grave
	limit T      // the largest ID that is given
	next  T      // the largest ID that was ever given: 0 when none was
	clock uint64 // how many steps were taken

	grave   queue[resting[T]] // in the order the IDs were given back, which is the order they leave
	free    queue[T]          // in the order they left the grave
	inGrave map[T]struct{}
	inFree  map[T]struct{}
}

// New makes an allocator that gives the IDs from 1 to limit, and keeps an ID that was given back in
// the grave for steps steps.
func New[T ID](steps uint64, limit T) Allocator[T] {
	return Allocator[T]{
		steps: steps, limit: limit, inGrave: map[T]struct{}{}, inFree: map[T]struct{}{},
	}
}

// Steps is how many steps an ID waits in the grave.
func (a *Allocator[T]) Steps() uint64 { return a.steps }

// Limit is the largest ID that the allocator gives.
func (a *Allocator[T]) Limit() T { return a.limit }

// Largest is the largest ID that the allocator ever gave, or 0.
func (a *Allocator[T]) Largest() T { return a.next }

// Clock is how many steps the allocator has taken.
func (a *Allocator[T]) Clock() uint64 { return a.clock }

// LiveCount is how many IDs were given and not given back.
func (a *Allocator[T]) LiveCount() int { return int(a.next) - a.grave.len() - a.free.len() }

// Give returns an ID that is live from now on: the one that has been free longest, or else the
// next one it never gave. It reports false when there is none, which is when every ID up to the
// limit is live or in the grave.
func (a *Allocator[T]) Give() (T, bool) {
	if id, ok := a.free.pop(); ok {
		delete(a.inFree, id)
		return id, true
	}
	if a.next >= a.limit {
		return 0, false
	}
	a.next++
	return a.next, true
}

// Release gives an ID back: it waits in the grave for the steps and is then free. It reports false
// for an ID that is not live, which changes nothing.
func (a *Allocator[T]) Release(id T) bool {
	if a.Status(id) != Live {
		return false
	}
	if a.steps == 0 {
		a.free.push(id)
		a.inFree[id] = struct{}{}
		return true
	}
	a.grave.push(resting[T]{id: id, until: a.clock + a.steps})
	a.inGrave[id] = struct{}{}
	return true
}

// Step takes every ID in the grave one step: the ones that have waited the steps leave it and are
// free.
func (a *Allocator[T]) Step() {
	a.clock++
	for {
		r, ok := a.grave.peek()
		if !ok || r.until > a.clock {
			return
		}
		a.grave.pop()
		delete(a.inGrave, r.id)
		a.free.push(r.id)
		a.inFree[r.id] = struct{}{}
	}
}

// Status says what an ID is.
func (a *Allocator[T]) Status(id T) Status {
	switch {
	case id == 0 || id > a.next:
		return NeverGiven
	case a.has(a.inGrave, id):
		return InGrave
	case a.has(a.inFree, id):
		return Free
	}
	return Live
}

func (a *Allocator[T]) has(set map[T]struct{}, id T) bool {
	_, ok := set[id]
	return ok
}

// Grave walks the IDs in the grave, in the order they were given back, with the step at which each
// leaves.
func (a *Allocator[T]) Grave() iter.Seq2[T, uint64] {
	return func(yield func(T, uint64) bool) {
		for r := range a.grave.all() {
			if !yield(r.id, r.until) {
				return
			}
		}
	}
}

// FreeIDs walks the IDs that are free, in the order they will be given.
func (a *Allocator[T]) FreeIDs() iter.Seq[T] { return a.free.all() }

// Clone returns an allocator that shares nothing with a.
func (a *Allocator[T]) Clone() Allocator[T] {
	c := Allocator[T]{
		steps: a.steps, limit: a.limit, next: a.next, clock: a.clock,
		grave: a.grave.clone(), free: a.free.clone(),
		inGrave: make(map[T]struct{}, len(a.inGrave)), inFree: make(map[T]struct{}, len(a.inFree)),
	}
	for id := range a.inGrave {
		c.inGrave[id] = struct{}{}
	}
	for id := range a.inFree {
		c.inFree[id] = struct{}{}
	}
	return c
}

// Encode writes the allocator.
func (a *Allocator[T]) Encode(w *binary.Writer) {
	w.Uvarint(a.steps)
	w.Uvarint(uint64(a.limit))
	w.Uvarint(uint64(a.next))
	w.Uvarint(a.clock)
	w.Uvarint(uint64(a.grave.len()))
	for r := range a.grave.all() {
		w.Uvarint(uint64(r.id))
		w.Uvarint(r.until)
	}
	w.Uvarint(uint64(a.free.len()))
	for id := range a.free.all() {
		w.Uvarint(uint64(id))
	}
}

// Decode reads what Encode wrote. It refuses an allocator that cannot be one: a count that claims
// more than the bytes left can hold, an ID that is 0, above what was given, in two places, or in
// the grave before its step.
func Decode[T ID](r *binary.Reader) (Allocator[T], error) {
	steps, limit := r.Uvarint(), T(r.Uvarint())
	a := New(steps, limit)
	a.next, a.clock = T(r.Uvarint()), r.Uvarint()
	if err := r.Err(); err != nil {
		return Allocator[T]{}, err
	}
	if a.next > a.limit {
		return Allocator[T]{}, fmt.Errorf("ids: %d given, and the limit is %d", a.next, a.limit)
	}
	n, err := count(r)
	if err != nil {
		return Allocator[T]{}, err
	}
	var last uint64
	for range n {
		id, until := T(r.Uvarint()), r.Uvarint()
		if err := a.checkNew(id); err != nil {
			return Allocator[T]{}, err
		}
		if until <= a.clock || until < last {
			return Allocator[T]{}, fmt.Errorf("ids: %d leaves the grave at step %d", id, until)
		}
		last = until
		a.grave.push(resting[T]{id: id, until: until})
		a.inGrave[id] = struct{}{}
	}
	if n, err = count(r); err != nil {
		return Allocator[T]{}, err
	}
	for range n {
		id := T(r.Uvarint())
		if err := a.checkNew(id); err != nil {
			return Allocator[T]{}, err
		}
		a.free.push(id)
		a.inFree[id] = struct{}{}
	}
	return a, r.Err()
}

// checkNew refuses an ID that is not one that was given, or that is already in the grave or free.
func (a *Allocator[T]) checkNew(id T) error {
	if id == 0 || id > a.next {
		return fmt.Errorf("ids: %d was never given", id)
	}
	if a.has(a.inGrave, id) || a.has(a.inFree, id) {
		return fmt.Errorf("ids: %d is in two places", id)
	}
	return nil
}

// count reads how many items follow, and refuses a number that claims more than the bytes left can
// hold: an item takes at least one byte.
func count(r *binary.Reader) (uint64, error) {
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return 0, err
	}
	if n > uint64(r.Left()) {
		return 0, fmt.Errorf("%w: %d items", binary.ErrShort, n)
	}
	return n, nil
}

// queue is a list that is added to at the end and taken from the front.
type queue[E any] struct {
	items []E
	head  int // where the front is in items
}

func (q *queue[E]) len() int { return len(q.items) - q.head }

func (q *queue[E]) push(e E) { q.items = append(q.items, e) }

func (q *queue[E]) peek() (E, bool) {
	if q.head == len(q.items) {
		var zero E
		return zero, false
	}
	return q.items[q.head], true
}

// pop takes the front. What was taken is let go of when more than half of the list is that.
func (q *queue[E]) pop() (E, bool) {
	e, ok := q.peek()
	if !ok {
		return e, false
	}
	var zero E
	q.items[q.head] = zero
	q.head++
	if q.head > 32 && q.head*2 > len(q.items) {
		q.items = slices.Clone(q.items[q.head:])
		q.head = 0
	}
	return e, true
}

func (q *queue[E]) all() iter.Seq[E] {
	return func(yield func(E) bool) {
		for _, e := range q.items[q.head:] {
			if !yield(e) {
				return
			}
		}
	}
}

func (q *queue[E]) clone() queue[E] { return queue[E]{items: slices.Clone(q.items[q.head:])} }
