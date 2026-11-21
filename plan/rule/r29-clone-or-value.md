# R29. Every piece of data in the simulation is a value or can be cloned (hard)

Every type held in a state of the simulation (`sim/state/...`) is one of two things:

- **A value type**: it holds no pointer, slice, map, channel or function, so copying it copies everything it contains.
- **A type with a `Clone` method** that copies everything it contains, and that the `Clone` of the state calls.

Nothing in a state is shared with another state, with an engine or with a published copy. The copy that an engine publishes at the end of a tick must stay exactly as it was while any other engine reads it.

An interface held in a state (such as `Entity`) is implemented only by value types. A change builds a new value and replaces the old one in the container; it never changes a value in place through a pointer.

A test per state checks the rule: change the state after `Clone` and the copy stays the same, and the other way round.

Why: the published copy is read by other goroutines while the owner goes on with the next tick, so any sharing is a data race, and it also breaks a replay.
