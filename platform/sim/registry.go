package sim

import "github.com/huykingsofm/project-b/platform/config"

// StableReader is what a registry reads the states of the other engines with: the latest state
// that each engine published. A state read from it is a copy that its engine will never change
// again, so it may be read without a lock and kept; it may be a few ticks old. It returns
// ErrStateNotFound for an engine that published none.
type StableReader interface {
	Read(id StateID) (State, error)
}

// Outbox is what an engine wants to send in a tick. Nothing is sent while the actions run: the
// engine sends the messages, in the order they were put here, when the plan is done.
type Outbox struct{ messages []Message }

// Add puts a message in the outbox.
func (o *Outbox) Add(l Message) { o.messages = append(o.messages, l) }

// Take returns the messages and empties the outbox.
func (o *Outbox) Take() []Message {
	out := o.messages
	o.messages = nil
	return out
}

// ScheduledAction is an action with the tick it is set for.
type ScheduledAction[S State] struct {
	At     uint64
	Action Action[S]
}

// Registry is what an action of a state of type S is given besides the state: what it wants done
// outside the state of its engine, and how to read what the other engines published. The engine
// that owns the state reads it when the plan is done.
type Registry[S State] struct {
	Outbox
	// Me is the ID of the state of the engine.
	Me StateID
	// Tick is the tick that the registry is used at.
	Tick uint64
	// Stable reads the states of the other engines.
	Stable StableReader

	scheduledActions []ScheduledAction[S]

	directives []DirectiveRequest
}

// Config returns the configuration that the engine of the tick reads: the snapshot in the copy of
// the configuration engine that the engine first read in the tick, so every action of the tick sees
// one configuration, or the defaults when there is none.
func (r *Registry[S]) Config() config.ReadOnlySnapshot {
	if r.Stable != nil {
		if cs, err := r.Get[*ConfigState](ConfigStateID); err == nil {
			return cs.Snapshot()
		}
	}
	return config.Defaults()
}

// Begin sets the tick that the registry is used at.
func (r *Registry[S]) Begin(tick uint64) { r.Tick = tick }

// Send puts a message for the state with the ID in the outbox. Its actions are actions of the
// type T of that state, so an action cannot be sent to a state it was not written for.
func (r *Registry[S]) Send[T State](to StateID, onFail OnFail, actions ...Action[T]) {
	r.Add(NewActionMessage(to, onFail, actions...))
}

// Get returns the latest state that the engine with the ID published, as a T: the Go type of the
// state, or an interface of its read methods, which is how an action is kept from changing a state
// that is not its own. It returns ErrStateNotFound when no state of the ID was published, and
// ErrStateWrongType when the state is not a T. What it returns may be a few ticks old, and the
// first state of an ID that an engine reads in a tick is the one it reads again until the tick is
// over.
func (r *Registry[S]) Get[T any](id StateID) (T, error) {
	var zero T
	got, err := r.Stable.Read(id)
	if err != nil {
		return zero, err
	}
	typed, ok := any(got).(T)
	if !ok {
		return zero, ErrStateWrongType
	}
	return typed, nil
}

// ToSelf sets an action to run in the next tick of the engine. It is not sent through a channel:
// the engine puts it in its scheduler when the tick is over.
func (r *Registry[S]) ToSelf(a Action[S]) { r.ToSelfAt(r.Tick+1, a) }

// ToSelfAt sets an action to run at a tick of the engine. An action that knows when it may run, a
// wait for a later moment, is set for exactly that tick and not tried again at every tick
// in between. A tick that is not in the future is the next one.
func (r *Registry[S]) ToSelfAt(tick uint64, a Action[S]) {
	r.scheduledActions = append(r.scheduledActions, ScheduledAction[S]{At: tick, Action: a})
}

// RequestCreateEngine asks the cluster to make an engine of a state of type T from the state and
// the actions that it runs once at its first tick (an action that is to run again sets itself
// again). The state has no ID: the cluster gives the engine one. Nothing is done while the actions
// run: the engine hands the request to its cluster when the tick is done, before it sends the
// messages of the tick, and the cluster answers it with a directive that the state of this
// registry takes at the start of a tick (DirectiveHandler), with the tag. The state that is to
// list the new engine is changed by that answer, as for any other change of a state (R28): the
// request only makes the engine. The state is the engine's from now on: the caller does not
// change it after.
func (r *Registry[S]) RequestCreateEngine[T State](tag []byte, st T, start ...Action[T]) {
	actions := make([]BaseAction, len(start))
	for i, a := range start {
		actions[i] = a
	}
	r.directives = append(r.directives, DirectiveRequest{
		Kind: DirectiveCreateEngine, Tag: tag, State: st, StartActions: actions,
	})
}

// RemoveEngine asks the cluster to remove the engine with the ID, and to answer with the tag.
// Nothing is done while the actions run: the engine hands the request to its cluster when the tick
// is done, and the cluster removes the engine between two ticks of it. The state that holds the ID
// is changed by the action, as for any other change of a state (R28): the request only ends the
// engine.
func (r *Registry[S]) RemoveEngine(id StateID, tag []byte) {
	r.directives = append(r.directives, DirectiveRequest{
		Kind: DirectiveRemoveEngine, Tag: tag, Target: id,
	})
}

// TakeDirectiveRequests returns what the actions of this tick asked of the cluster, in the order
// they asked, and forgets it.
func (r *Registry[S]) TakeDirectiveRequests() []DirectiveRequest {
	out := r.directives
	r.directives = nil
	return out
}

// TakeSelf returns what the engine set for itself in this tick, in the order it was set.
func (r *Registry[S]) TakeSelf() []ScheduledAction[S] {
	self := r.scheduledActions
	r.scheduledActions = nil
	return self
}
