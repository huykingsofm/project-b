package action

import (
	"fmt"
	"iter"
	"sort"

	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/shared/logging"
)

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
	buckets   map[ActionKey][]*PlanEntry[A]
	cancelled []*PlanEntry[A]
}

// NewPlan returns an empty plan, with the default configuration until Reset gives it one.
func NewPlan[A Action]() *Plan[A] {
	return &Plan[A]{buckets: map[ActionKey][]*PlanEntry[A]{}, cfg: config.Defaults()}
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
	byID := ActionKey{ID: id}
	p.buckets[byID] = append(p.buckets[byID], e)
	if who, ok := any(a).(Acting); ok {
		byActor := ActionKey{Actor: who.Actor(), ID: id}
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
func (p *Plan[A]) Find(k ActionKey) []*PlanEntry[A] {
	var out []*PlanEntry[A]
	for _, e := range p.buckets[k] {
		if !e.ran && !e.cancelled {
			out = append(out, e)
		}
	}
	return out
}

// FindByID returns the entries of the actions of one ID that have not run and are not cancelled, in
// the order they were added to the plan, each with its action as a T: for _, damage := range
// p.FindByID[*DamageAction](DamageActionID). It is how an effect finds the actions it changes
// without asserting a type itself.
//
// It is lazy: an entry is looked at when the loop asks for it, so a loop that stops early does not
// walk the rest, and it makes no slice, so a walk inside another walk costs nothing more. An entry
// that was cancelled before the loop reached it is left out. Cancel in the loop is safe; do not
// Append to the plan in it, because the walk goes on over the entries that were there when it
// began. An entry whose action is not a T is a mistake of the caller: it is logged and left out.
func (p *Plan[A]) FindByID[T any](id ID) iter.Seq2[*PlanEntry[A], T] {
	return func(yield func(*PlanEntry[A], T) bool) {
		for _, e := range p.buckets[ActionKey{ID: id}] {
			if e.ran || e.cancelled {
				continue
			}
			a, ok := any(e.action).(T)
			if !ok {
				logging.Error(
					"plan: an action of the ID asked for is not of the type asked for",
					"id", id, "action", fmt.Sprintf("%T", e.action),
					"want", fmt.Sprintf("%T", a),
				)
				continue
			}
			if !yield(e, a) {
				return
			}
		}
	}
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
