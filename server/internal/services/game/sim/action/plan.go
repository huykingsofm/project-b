package action

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/shared/logging"
)

// PlanKey names a set of actions that a plan keeps an index of: the actions of one ID that one
// actor performs, or, with a nil Actor, all the actions of one ID; or, with Type set and nothing
// else, the actions of one Go type.
type PlanKey struct {
	Actor any
	ID    ID
	Type  reflect.Type
}

// PlanEntry is an action in a plan, with what the plan knows of it.
type PlanEntry[A Action] struct {
	action    A
	depth     uint8 // how many actions deep the chain that made it is; 0 for a command or a message
	ran       bool
	cancelled bool
}

// Action returns the action of the entry.
func (e *PlanEntry[A]) Action() A { return e.action }

// Plan is the list of actions an engine runs in a tick, sorted by priority (smaller first) and, for
// equal priorities, by arrival. It has a cursor: whatever is added while it runs goes after the
// entry that runs, so what has run is never disturbed.
//
// An action that runs can look at the actions behind it (Find) and cancel them (Cancel). The plan
// is indexed by the ID of its actions and by the actor that performs them, so finding "the damage
// in this tick" does not walk the whole plan: this is the pattern that every effect that reacts to
// another action uses.
type Plan[A Action] struct {
	entries   []*PlanEntry[A]
	next      int              // entries[:next] have been returned by Next
	current   *PlanEntry[A]    // the entry that runs, nil before the first
	cfg       *config.Snapshot // the configuration of the tick, which says how deep a chain may go
	tooDeep   []*PlanEntry[A]
	buckets   map[PlanKey][]*PlanEntry[A]
	cancelled []*PlanEntry[A]
}

// NewPlan returns an empty plan, with the default configuration until Reset gives it one.
func NewPlan[A Action]() *Plan[A] {
	return &Plan[A]{buckets: map[PlanKey][]*PlanEntry[A]{}, cfg: config.Defaults()}
}

// Reset empties the plan, puts the cursor before the first entry, and keeps the configuration of
// the tick: the plan reads from it how many actions deep a chain may go.
func (p *Plan[A]) Reset(cfg *config.Snapshot) {
	clear(p.entries)
	p.entries = p.entries[:0]
	p.next = 0
	p.current = nil
	p.cfg = cfg
	clear(p.tooDeep)
	p.tooDeep = p.tooDeep[:0]
	clear(p.buckets)
	clear(p.cancelled)
	p.cancelled = p.cancelled[:0]
}

// Len returns how many entries the plan holds, run or not.
func (p *Plan[A]) Len() int { return len(p.entries) }

// Append adds an action in its place: after the last entry whose priority is not larger, and never
// before the entry that runs. Its depth is one more than the depth of the entry that runs, or 0
// when none runs (a command, a message, what an engine set for itself).
func (p *Plan[A]) Append(a A) {
	e := &PlanEntry[A]{action: a}
	if p.current != nil {
		e.depth = p.current.depth + 1
	}
	rest := p.entries[p.next:]
	at := p.next + sort.Search(
		len(rest), func(i int) bool { return rest[i].action.Priority() > a.Priority() },
	)
	p.entries = append(p.entries, nil)
	copy(p.entries[at+1:], p.entries[at:])
	p.entries[at] = e

	id := a.ID()
	byID := PlanKey{ID: id}
	p.buckets[byID] = append(p.buckets[byID], e)
	byType := PlanKey{Type: reflect.TypeOf(a)}
	p.buckets[byType] = append(p.buckets[byType], e)
	if who, ok := any(a).(Acting); ok {
		byActor := PlanKey{Actor: who.Actor(), ID: id}
		p.buckets[byActor] = append(p.buckets[byActor], e)
	}
}

// Next moves the cursor to the next entry that is not cancelled and is not too deep, and returns
// it. An entry that is too deep is cut: it does not run and is listed in TooDeep.
func (p *Plan[A]) Next() (*PlanEntry[A], bool) {
	for p.next < len(p.entries) {
		e := p.entries[p.next]
		p.next++
		if e.cancelled {
			continue
		}
		if int(e.depth) > MaxDepth.Get(p.cfg) {
			e.cancelled = true
			p.tooDeep = append(p.tooDeep, e)
			logging.Warn(
				"action cut: its chain is too deep",
				"action",
				fmt.Sprintf("%T", e.action),
				"depth",
				e.depth,
				"max_depth",
				MaxDepth.Get(p.cfg),
			)
			continue
		}
		e.ran = true
		p.current = e
		return e, true
	}
	return nil, false
}

// Find returns the entries under the key that have not run and are not cancelled, in the order
// they were added to the plan.
func (p *Plan[A]) Find(k PlanKey) []*PlanEntry[A] {
	var out []*PlanEntry[A]
	for _, e := range p.buckets[k] {
		if !e.ran && !e.cancelled {
			out = append(out, e)
		}
	}
	return out
}

// Hit is an entry of a plan, with its action as the type that was asked for.
type Hit[A Action, T any] struct {
	Entry  *PlanEntry[A]
	Action T
}

// FindType returns the entries that have not run and are not cancelled and whose action is a T, in
// the order they were added to the plan. T is the Go type of the action as it was added (for an
// action that is a pointer, the pointer type): p.FindType[*DamageAction](). It is the way an effect
// finds the actions it changes without an ID value and without asserting a type itself.
func (p *Plan[A]) FindType[T any]() []Hit[A, T] {
	var out []Hit[A, T]
	for _, e := range p.buckets[PlanKey{Type: reflect.TypeFor[T]()}] {
		if e.ran || e.cancelled {
			continue
		}
		if a, ok := any(e.action).(T); ok {
			out = append(out, Hit[A, T]{Entry: e, Action: a})
		}
	}
	return out
}

// Cancel stops an entry that has not run from running.
func (p *Plan[A]) Cancel(e *PlanEntry[A]) {
	if e.ran || e.cancelled {
		return
	}
	e.cancelled = true
	p.cancelled = append(p.cancelled, e)
}

// TooDeep returns the entries that were cut in this tick because their chain went too deep.
func (p *Plan[A]) TooDeep() []*PlanEntry[A] { return p.tooDeep }

// Cancelled returns the entries that were cancelled in this tick, so that the engine can tell their
// actions that they did not run.
func (p *Plan[A]) Cancelled() []*PlanEntry[A] { return p.cancelled }
