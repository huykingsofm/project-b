// Package ids gives out the IDs of things that come and go while a game runs, and keeps an ID that
// was given back in a grave for a number of steps before it can be given again, so that what still
// points at the old one has had the time to let go.
//
// An allocator never gives 0. It gives the IDs in order, from 1 up to its limit, and then goes
// round to 1 again and skips an ID that is live or in the grave. It holds where it is in that
// order, the IDs that are live, and the grave: an ID that left the grave is the same to it as one
// it never gave, so it holds no list of them, and it is as big as what is live and what waits,
// whatever was given before. A checkpoint writes it without the live IDs, which are the IDs of
// what the owner holds: the owner marks them live again as it reads them (MarkLive), so the two
// cannot differ. A replay of the same calls ends with the same allocator. It is not safe for
// concurrent use: the state that owns it is changed by one goroutine.
package ids

import (
	"fmt"
	"iter"
	"slices"

	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ID is what an allocator gives out: a number.
type ID interface{ ~uint32 | ~uint64 }

// Status says what an ID is.
type Status uint8

const (
	// NotGiven is an ID that is neither live nor in the grave: one never given, one that left the
	// grave, and 0, which is never given.
	NotGiven Status = iota + 1
	// Live is an ID that was given and not given back.
	Live
	// InGrave is an ID that was given back and has not waited the steps yet.
	InGrave
)

// String is the name of the status.
func (s Status) String() string {
	switch s {
	case NotGiven:
		return "not given"
	case Live:
		return "live"
	case InGrave:
		return "in the grave"
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
	steps   uint64 // how many steps an ID waits in the grave
	limit   T      // the largest ID that is given
	last    T      // the ID given last: the next one is looked for after it; 0 when none was
	wrapped bool   // whether it went round to 1: until then every ID above last was never given
	clock   uint64 // how many steps were taken

	live map[T]struct{}

	grave   queue[resting[T]] // in the order the IDs were given back, which is the order they leave
	inGrave map[T]struct{}
}

// New makes an allocator that gives the IDs from 1 to limit, and keeps an ID that was given back in
// the grave for steps steps.
func New[T ID](steps uint64, limit T) Allocator[T] {
	return Allocator[T]{
		steps: steps, limit: limit, live: map[T]struct{}{}, inGrave: map[T]struct{}{},
	}
}

// Steps is how many steps an ID waits in the grave.
func (a *Allocator[T]) Steps() uint64 { return a.steps }

// Limit is the largest ID that the allocator gives.
func (a *Allocator[T]) Limit() T { return a.limit }

// Last is the ID given last, or 0 when none was.
func (a *Allocator[T]) Last() T { return a.last }

// Wrapped reports whether the allocator went round to 1 after its limit.
func (a *Allocator[T]) Wrapped() bool { return a.wrapped }

// Clock is how many steps the allocator has taken.
func (a *Allocator[T]) Clock() uint64 { return a.clock }

// LiveCount is how many IDs are live.
func (a *Allocator[T]) LiveCount() int { return len(a.live) }

// GraveLen is how many IDs are in the grave.
func (a *Allocator[T]) GraveLen() int { return a.grave.len() }

// Give returns an ID that is live from now on: the next one after the last one given that is
// neither live nor in the grave, going round to 1 after the limit. It reports false when there is
// none, which is when every ID up to the limit is live or in the grave.
func (a *Allocator[T]) Give() (T, bool) {
	if uint64(len(a.live))+uint64(a.grave.len()) >= uint64(a.limit) {
		return 0, false
	}
	for {
		if a.last == a.limit {
			a.last, a.wrapped = 0, true
		}
		a.last++
		if a.wrapped && (a.IsLive(a.last) || a.InGrave(a.last)) {
			continue // there is one that is neither: the count above says so
		}
		a.live[a.last] = struct{}{}
		return a.last, true
	}
}

// Release gives a live ID back: it waits in the grave for the steps, and is then not given. It
// reports false for an ID that is not live, which changes nothing.
func (a *Allocator[T]) Release(id T) bool {
	if !a.IsLive(id) {
		return false
	}
	delete(a.live, id)
	if a.steps == 0 {
		return true
	}
	a.grave.push(resting[T]{id: id, until: a.clock + a.steps})
	a.inGrave[id] = struct{}{}
	return true
}

// Step takes every ID in the grave one step: the ones that have waited the steps leave it.
func (a *Allocator[T]) Step() {
	a.clock++
	for {
		r, ok := a.grave.peek()
		if !ok || r.until > a.clock {
			return
		}
		a.grave.pop()
		delete(a.inGrave, r.id)
	}
}

// IsLive reports whether an ID was given and not given back.
func (a *Allocator[T]) IsLive(id T) bool {
	_, ok := a.live[id]
	return ok
}

// InGrave reports whether an ID is in the grave.
func (a *Allocator[T]) InGrave(id T) bool {
	_, ok := a.inGrave[id]
	return ok
}

// Status says what an ID is.
func (a *Allocator[T]) Status(id T) Status {
	switch {
	case a.IsLive(id):
		return Live
	case a.InGrave(id):
		return InGrave
	}
	return NotGiven
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

// Clone returns an allocator that shares nothing with a.
func (a *Allocator[T]) Clone() Allocator[T] {
	c := Allocator[T]{
		steps: a.steps, limit: a.limit, last: a.last, wrapped: a.wrapped, clock: a.clock,
		live:  make(map[T]struct{}, len(a.live)),
		grave: a.grave.clone(), inGrave: make(map[T]struct{}, len(a.inGrave)),
	}
	for id := range a.live {
		c.live[id] = struct{}{}
	}
	for id := range a.inGrave {
		c.inGrave[id] = struct{}{}
	}
	return c
}

// Encode writes the allocator.
func (a *Allocator[T]) Encode(w *binary.Writer) {
	w.Uvarint(a.steps)
	w.Uvarint(uint64(a.limit))
	w.Uvarint(uint64(a.last))
	w.Bool(a.wrapped)
	w.Uvarint(a.clock)
	w.Uvarint(uint64(a.grave.len()))
	for r := range a.grave.all() {
		w.Uvarint(uint64(r.id))
		w.Uvarint(r.until)
	}
}

// Decode reads what Encode wrote, with no live ID: the owner marks each of its IDs live with
// MarkLive. It refuses an allocator that cannot be one: a count that claims more than the bytes
// left can hold, the last ID above the limit, an ID in the grave that is 0, never given or there
// twice, and one whose step is not after the clock and the ones before it.
func Decode[T ID](r *binary.Reader) (Allocator[T], error) {
	steps, limit := r.Uvarint(), T(r.Uvarint())
	a := New(steps, limit)
	a.last, a.wrapped, a.clock = T(r.Uvarint()), r.Bool(), r.Uvarint()
	if err := r.Err(); err != nil {
		return Allocator[T]{}, err
	}
	if a.last > a.limit {
		return Allocator[T]{}, fmt.Errorf(
			"ids: %d was given last, and the limit is %d", a.last, limit,
		)
	}
	n, err := count(r)
	if err != nil {
		return Allocator[T]{}, err
	}
	var last uint64
	for range n {
		id, until := T(r.Uvarint()), r.Uvarint()
		if err := r.Err(); err != nil {
			return Allocator[T]{}, err
		}
		if !a.mayHold(id) {
			return Allocator[T]{}, fmt.Errorf(
				"ids: %d in the grave was never given or is held twice", id,
			)
		}
		if until <= a.clock || until < last {
			return Allocator[T]{}, fmt.Errorf("ids: %d leaves the grave at step %d", id, until)
		}
		last = until
		a.grave.push(resting[T]{id: id, until: until})
		a.inGrave[id] = struct{}{}
	}
	return a, nil
}

// MarkLive makes an ID live again after Decode: the owner calls it for each ID it holds, as it
// reads what it holds. It refuses an ID that the allocator cannot have given and still be
// holding: 0, above the limit, never given, in the grave, or marked already.
func (a *Allocator[T]) MarkLive(id T) error {
	if !a.mayHold(id) {
		return fmt.Errorf(
			"ids: %d cannot be live: it is 0, never given, in the grave or live already", id,
		)
	}
	a.live[id] = struct{}{}
	return nil
}

// mayHold reports whether an ID can be one that the allocator gave and does not hold yet.
func (a *Allocator[T]) mayHold(id T) bool {
	given := id != 0 && id <= a.limit && (a.wrapped || id <= a.last)
	return given && !a.IsLive(id) && !a.InGrave(id)
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
