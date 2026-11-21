package state

// Events is the events of a tick, in the order they happened: what the clients of a scope should be
// told. It is the second region of a state: it is not data, it is not cloned and it is not published.
// The three scopes share this one container; each scope has its own type of event (E), so an event of
// one scope cannot be put in the events of another.
type Events[E any] struct{ events []E }

// Add appends an event.
func (e *Events[E]) Add(event E) { e.events = append(e.events, event) }

// Len returns how many events there are.
func (e *Events[E]) Len() int { return len(e.events) }

// Take returns the events and leaves it empty.
func (e *Events[E]) Take() []E {
	out := e.events
	e.events = nil
	return out
}
