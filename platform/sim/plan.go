package sim

import (
	"fmt"
	"iter"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/shared/logging"
	"github.com/huykingsofm/project-b/shared/table"
)

// PlanEntry is an action in a plan, with what the plan knows of it.
type PlanEntry[S State] struct {
	action Action[S]
	depth  uint8 // how many actions deep the chain that made it is; 0 for a command or a message
	key    int64 // where the queue of the plan holds it
}

// Action returns the action of the entry.
func (e *PlanEntry[S]) Action() Action[S] { return e.action }

// Plan is the list of actions an engine runs in a tick, sorted by priority (smaller first) and, for
// equal priorities, by arrival. It is a table.PriorityQueue: whatever is added while it runs goes
// after the entry that runs, so what has run is never disturbed.
//
// An action that runs can look at the actions behind it (Find) and cancel them (Cancel). The queue
// has a column for the ID of its actions and a column for the actor that performs them, so finding
// "the damage in this tick" does not walk the whole plan: this is the pattern that every effect
// that reacts to another action uses. What a plan adds to the queue is the depth of a chain of
// actions, and the entries that were cut because it went too deep.
type Plan[S State] struct {
	queue     *table.PriorityQueue[*PlanEntry[S], int16]
	current   *PlanEntry[S]    // the entry that runs, nil before the first
	cfg       *config.Snapshot // the configuration of the tick, which says how deep a chain may go
	tooDeep   []*PlanEntry[S]
	cancelled []*PlanEntry[S] // made by Cancelled from what the queue says
	built     []PlanEntry[S]  // the entries of Build, kept for the next tick
	built2    []*PlanEntry[S]
}

// NewPlan returns an empty plan, with the default configuration until Reset gives it one.
func NewPlan[S State]() *Plan[S] {
	p := &Plan[S]{
		queue: table.NewPriorityQueue(func(e *PlanEntry[S]) int16 { return e.action.Priority() }),
		cfg:   config.Defaults(),
	}
	// the columns: by the ID of the action, and by the actor and the ID of an action that has one
	err := p.queue.Index(func(e *PlanEntry[S]) (ActionID, bool) { return e.action.ID(), true })
	if err == nil {
		err = p.queue.Index(func(e *PlanEntry[S]) (ActionKey, bool) {
			who, ok := any(e.action).(Acting)
			if !ok {
				return ActionKey{}, false
			}
			return ActionKey{Actor: who.Actor(), ID: e.action.ID()}, true
		})
	}
	if err != nil {
		logging.Error("plan: the columns were not declared", "err", err)
	}
	return p
}

// Reset empties the plan, puts the cursor before the first entry, and keeps the configuration of
// the tick: the plan reads from it how many actions deep a chain may go.
func (p *Plan[S]) Reset(cfg *config.Snapshot) {
	p.queue.Reset()
	p.current = nil
	p.cfg = cfg
	clear(p.tooDeep)
	p.tooDeep = p.tooDeep[:0]
	clear(p.cancelled)
	p.cancelled = p.cancelled[:0]
	clear(p.built)
	clear(p.built2)
}

// Len returns how many entries the plan holds, run or not.
func (p *Plan[S]) Len() int { return p.queue.Len() }

// Build makes the plan hold the actions, which are the ones a tick starts with (a command, a
// message, what an engine set for itself), at depth 0, and takes out what it held. It is what an
// Append of each action makes, with the order done once.
func (p *Plan[S]) Build(actions []Action[S]) {
	p.Reset(p.cfg)
	if cap(p.built) < len(actions) {
		p.built = make([]PlanEntry[S], len(actions))
	}
	p.built = p.built[:len(actions)]
	p.built2 = p.built2[:0]
	for i, a := range actions {
		p.built[i] = PlanEntry[S]{action: a, key: int64(i)}
		p.built2 = append(p.built2, &p.built[i])
	}
	p.queue.BuildFrom(p.built2)
}

// Append adds an action in its place: after the last entry whose priority is not larger, and never
// before the entry that runs. Its depth is one more than the depth of the entry that runs, or 0
// when none runs (a command, a message, what an engine set for itself).
func (p *Plan[S]) Append(a Action[S]) {
	e := &PlanEntry[S]{action: a}
	if p.current != nil {
		e.depth = p.current.depth + 1
	}
	e.key = p.queue.Add(e)
}

// Next moves the cursor to the next entry that is not cancelled and is not too deep, and returns
// it. An entry that is too deep is cut: it does not run and is listed in TooDeep.
func (p *Plan[S]) Next() (*PlanEntry[S], bool) {
	for {
		_, e, ok := p.queue.Next()
		if !ok {
			return nil, false
		}
		if int(e.depth) > MaxDepth.Get(p.cfg) {
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
		p.current = e
		return e, true
	}
}

// Find returns the entries under the key that have not run and are not cancelled, in the order
// they were added to the plan. A key with no actor names all the actions of its ID.
func (p *Plan[S]) Find(k ActionKey) []*PlanEntry[S] {
	var out []*PlanEntry[S]
	if k.Actor == nil {
		for _, e := range p.queue.Lookup(k.ID) {
			out = append(out, e)
		}
		return out
	}
	for _, e := range p.queue.Lookup(k) {
		out = append(out, e)
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

// Cancel stops an entry that has not run from running.
func (p *Plan[S]) Cancel(e *PlanEntry[S]) { p.queue.Cancel(e.key) }

// TooDeep returns the entries that were cut in this tick because their chain went too deep.
func (p *Plan[S]) TooDeep() []*PlanEntry[S] { return p.tooDeep }

// Cancelled returns the entries that were cancelled in this tick, so that the engine can tell their
// actions that they did not run.
func (p *Plan[S]) Cancelled() []*PlanEntry[S] {
	p.cancelled = p.cancelled[:0]
	for _, key := range p.queue.Cancelled() {
		e, _ := p.queue.Get(key)
		p.cancelled = append(p.cancelled, e)
	}
	return p.cancelled
}
