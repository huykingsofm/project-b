package sim

import (
	"fmt"
	"iter"

	"github.com/huykingsofm/project-b/shared/logging"
	"github.com/huykingsofm/project-b/shared/table"
)

const (
	EntryPending   EntryStatus = iota // the action is still to run
	EntryRan                          // the action ran
	EntryCancelled                    // the action was cancelled before it ran
	EntryCut                          // the action was cut: its chain went too deep
)

// EntryStatus says where an entry of a plan stands: whether its action is still to run, and if not,
// why.
type EntryStatus uint8

// Plan is the list of actions an engine runs in a tick, sorted by priority (smaller first) and, for
// equal priorities, by arrival. It is a table.PriorityQueue: whatever is added while it runs goes
// after the entry that runs, so what has run is never disturbed.
//
// An action that runs can look at the other actions of the plan (FindByID), the ones that ran and
// were cancelled as well as the ones to run, and cancel those that are still to run (Cancel). The
// queue has a column for the ID of its actions, so finding
// "the damage in this tick" does not walk the whole plan: this is the pattern that every effect
// that reacts to another action uses. What a plan adds to the queue is the depth of a chain of
// actions, and the entries that were cut because it went too deep.
type Plan[S State] struct {
	queue     *table.PriorityQueue[*PlanEntry[S], int16]
	current   *PlanEntry[S]   // the entry that runs, nil before the first
	cancelled []*PlanEntry[S] // in the order they were cancelled
	entries   []PlanEntry[S]  // the entries of Build, kept for the next tick
	entryPtrs []*PlanEntry[S] // pointers to entries, which the queue is built from
}

// NewPlan returns an empty plan.
func NewPlan[S State]() *Plan[S] {
	p := &Plan[S]{
		queue: &table.PriorityQueue[*PlanEntry[S], int16]{},
	}
	// the column: by the ID of the action
	err := p.queue.Index(func(e *PlanEntry[S]) (ActionID, bool) { return e.action.ID(), true })
	if err != nil {
		logging.Error("plan: the columns were not declared", "err", err)
	}
	return p
}

// Reset empties the plan and puts the cursor before the first entry.
func (p *Plan[S]) Reset() {
	p.queue.Reset()
	p.current = nil
	clear(p.cancelled)
	p.cancelled = p.cancelled[:0]
	clear(p.entries)
	clear(p.entryPtrs)
}

// Len returns how many entries the plan holds, run or not.
func (p *Plan[S]) Len() int { return p.queue.Len() }

// Build makes the plan hold the actions, which are the ones a tick starts with (a command, a
// message, what an engine set for itself), at depth 0, and takes out what it held. It is what an
// Append of each action makes, with the order done once.
func (p *Plan[S]) Build(actions []Action[S]) {
	p.Reset()
	if cap(p.entries) < len(actions) {
		p.entries = make([]PlanEntry[S], len(actions))
	}
	p.entries = p.entries[:len(actions)]
	p.entryPtrs = p.entryPtrs[:0]
	for i, a := range actions {
		p.entries[i] = PlanEntry[S]{action: a}
		p.entryPtrs = append(p.entryPtrs, &p.entries[i])
	}
	p.queue.BuildFrom(p.entryPtrs)
}

// Append adds an action in its place: after the last entry whose priority is not larger, and never
// before the entry that runs. Its depth is one more than the depth of the entry that runs, or 0
// when none runs (a command, a message, what an engine set for itself).
func (p *Plan[S]) Append(a Action[S]) {
	e := &PlanEntry[S]{action: a}
	if p.current != nil {
		e.depth = p.current.depth + 1
	}
	p.queue.Add(e)
}

// Next moves the cursor to the next entry that is pending and is not too deep, and returns it, with
// its status set to ran. An entry that is cancelled is passed over. An entry is too deep when its
// depth is more than maxDepth (the depth of a command or a message is 0, and an action that an
// action adds is one deeper than it): it does not run, it is cut, and its status is EntryCut.
func (p *Plan[S]) Next(maxDepth int) (*PlanEntry[S], bool) {
	for {
		_, e, ok := p.queue.Next()
		if !ok {
			return nil, false
		}
		if e.status != EntryPending {
			continue // cancelled
		}
		if int(e.depth) > maxDepth {
			e.status = EntryCut
			logging.Warn(
				"action cut: its chain is too deep",
				"action",
				fmt.Sprintf("%T", e.action),
				"depth",
				e.depth,
				"max_depth",
				maxDepth,
			)
			continue
		}
		e.status = EntryRan
		p.current = e
		return e, true
	}
}

// FindByID returns the entries of the actions of one ID, in the order they were added to the plan,
// each with its action as a T: for e, damage := range p.FindByID[*DamageAction](DamageActionID). It
// gives the entries that ran and the ones that were cancelled as well as the ones still to run, so
// whoever changes an action checks first that the status of its entry is EntryPending. It is how an
// effect finds the actions it changes without asserting a type itself.
//
// It is lazy: an entry is looked at when the loop asks for it, so a loop that stops early does not
// walk the rest, and it makes no slice, so a walk inside another walk costs nothing more. Cancel in
// the loop is safe, and an entry cancelled before the loop reaches it is given with its new
// status; do not Append to the plan in it, because the walk goes on over the entries that were
// there when it began. An entry whose action is not a T is a mistake of the caller: it is logged
// and left out.
func (p *Plan[S]) FindByID[T any](id ActionID) iter.Seq2[*PlanEntry[S], T] {
	return func(yield func(*PlanEntry[S], T) bool) {
		for _, e := range p.queue.Lookup(id) {
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

// Cancel stops an entry that is still to run from running, and lists it in Cancelled. An entry that
// ran, was cancelled or was cut already is not changed.
func (p *Plan[S]) Cancel(e *PlanEntry[S]) {
	if e.status != EntryPending {
		return
	}
	e.status = EntryCancelled
	p.cancelled = append(p.cancelled, e)
}

// Cancelled returns the entries that were cancelled in this tick, in the order they were, so that
// the engine can tell their actions that they did not run.
func (p *Plan[S]) Cancelled() []*PlanEntry[S] { return p.cancelled }

// PlanEntry is an action in a plan, with what the plan knows of it.
type PlanEntry[S State] struct {
	action Action[S]
	depth  uint8 // how many actions deep the chain that made it is; 0 for a command or a message
	status EntryStatus
}

// Action returns the action of the entry.
func (e *PlanEntry[S]) Action() Action[S] { return e.action }

// Priority is the priority of the action of the entry, which orders the entry in the plan.
func (e *PlanEntry[S]) Priority() int16 { return e.action.Priority() }

// Status returns where the entry stands. FindByID gives the entries that ran or were
// cancelled as well as the ones to run, so whoever walks them checks the status of each.
func (e *PlanEntry[S]) Status() EntryStatus { return e.status }
