package sim

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

// Timed is an action with the tick it is set for.
type Timed[S State] struct {
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

	next    []Action[S]
	later   []Timed[S]
	cancels []Cancel
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
// the engine reads it when the next tick begins.
func (r *Registry[S]) ToSelf(a Action[S]) { r.next = append(r.next, a) }

// ToSelfAt sets an action to run at a tick of the engine. An action that knows when it may run, a
// wait for a later moment, is set for exactly that tick and not tried again at every tick
// in between.
func (r *Registry[S]) ToSelfAt(tick uint64, a Action[S]) {
	if tick <= r.Tick {
		r.next = append(r.next, a)
		return
	}
	r.later = append(r.later, Timed[S]{At: tick, Action: a})
}

// CancelWaiting ends sooner what waits in the engine: the actions the key names that wait for a
// later tick, or for the next one, and what this tick set before the call. It takes effect when
// the tick is over. It does not reach an action that is in the plan of this tick (Plan.Cancel
// does), nor what such an action sets again after the call, so it is not what stops an action
// that sets itself again: that action decides for itself, from the state, whether it still does
// anything and whether it sets itself again, and a cancel only spares the ticks in between.
func (r *Registry[S]) CancelWaiting(key ActionKey) {
	r.cancels = append(r.cancels, Cancel{Key: key, Next: len(r.next), Later: len(r.later)})
}

// TakeSelf returns what the engine set for itself in this tick.
func (r *Registry[S]) TakeSelf() ([]Action[S], []Timed[S]) {
	next, later := r.next, r.later
	r.next, r.later = nil, nil
	return next, later
}

// TakeCancels returns what the engine cancelled in this tick.
func (r *Registry[S]) TakeCancels() []Cancel {
	cancels := r.cancels
	r.cancels = nil
	return cancels
}
