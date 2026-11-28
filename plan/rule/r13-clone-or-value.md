# R13. Every piece of data in the simulation is a value or can be cloned (hard)

Every type held in a state of the simulation (`sim/state/...`) is one of two things:

- **A value type**: it holds no pointer, slice, map, channel or function, so copying it copies everything it contains.
- **A type with a `Clone` method** that copies everything it contains, and that the `Clone` of the state calls.

Nothing in a state is shared with another state, with an engine, with a published copy, or with whoever gave it to the state. The copy that an engine publishes at the end of a tick must stay exactly as it was while any other engine reads it.

An interface held in a state (such as `Entity`) is implemented only by value types. A change builds a new value and replaces the old one in the container; it never changes a value in place through a pointer.

## Containers

`stablemap`, `table`, `ring` and `bimap` copy their elements with `Clone` when the element type has one and by assignment when it has not (package `clone`). So the element type of a container is a value or has `Clone`: a container of a type that holds a slice or a map and has no `Clone` shares it with its copy.

## Coming in and going out

- **In:** a state method that stores something it was given copies it first (`AddCharacterEntity` clones the character). The caller keeps what it sent and the state never changes it.
- **Out:** a state gives out a copy, or what it removed from itself (R12: removed first, then sent). It never gives out a view of what it still holds and changes.

## Crossing between goroutines

What crosses from one goroutine to another, commands, actions, messages and events, is a value, or holds only values and types with `Clone`.

- Every command has `Clone`, and an engine calls it when it takes the command in, because the caller keeps the command it sent and may change it. A command that holds only values returns itself.
- Every action is a value type: its methods are on the value, not on a pointer, so no one changes an action through the interface that holds it, and an interface may be shared. An action that changes what it holds as it runs (a run of movement that reserved its time, a shield that started) changes its own copy and sets that copy for a later tick; it never changes itself in place. Its `Clone` returns itself, or copies what it holds that is not a value (a character).
- A state method that makes an event from what it was given copies what the event holds, because the event goes to the emitter on another goroutine.

## Tests

- A test per state: change the state after `Clone` and the copy stays the same, and the other way round.
- `aliasing.Disjoint` finds any map, pointer or slice array that a state and its clone, or a value that entered and the state that holds it, still share.
- `aliasing.CheckCloneable` checks the types: the types of each state, and every command, action and event. A new type that holds a slice or a map and has no `Clone` fails the test.
- The engines run with a caller that keeps and changes what it sent, under the race detector, and a run with every command copied through bytes must give the same hashes as one without.

Why: the published copy is read by other goroutines while the owner goes on with the next tick, and a command is kept by the goroutine that sent it, so any sharing is a data race, and it also breaks a replay.
