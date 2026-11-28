# R0. SOLID, above all open for extension (hard)

Every design follows the SOLID principles. This rule comes before all the others, and when it conflicts with another rule, the other rule is read in its light.

- **Open for extension, closed for modification (above all).** A new kind, a new use or a new provider is added by writing new code, not by changing code that exists and works. A mechanism therefore takes what varies as something it is given (a function, an interface, an entry that declares itself) and not as a branch inside it. A `switch` or a chain of conditionals over kinds, or a central list that must be edited for every new kind, is a sign that the design is closed to extension and has to be reshaped. Ask of every mechanism: what is the next thing that will be added, and can it be added without touching what is here? (This goes with [R4](r04-essentials-built-now.md): the shape of the mechanism is made for the uses to come.)
- **Single responsibility.** A type, a function and a file have one reason to change ([R7](r07-organization.md)).
- **Liskov substitution.** Every implementation of an interface keeps the whole contract of the interface, so that any of them can stand in for another without the caller knowing which it is.
- **Interface segregation.** An interface is small and has only what its caller needs ([R1](r01-layer-dependency.md)).
- **Dependency inversion.** Code depends on interfaces that the caller defines, not on concrete providers ([R1](r01-layer-dependency.md)).

Where the owner's own design fixes a central place on purpose (the action IDs are constants of one file), that is the owner's decision and stays; it is not a reason to add other central places.

## No action is treated specially

No action (Move, Attack, Chat, ...) has a field, a pattern, a part of the protocol or a code path of its own. When a need first shows up with one action, name the general concept and put the need there: a **gate** per character with `until`, `seq` and `done`, used by every action; each action decides its own rule for a busy gate through that one mechanism (it may wait its turn, wait up to a buffer, or be refused). When an action has to wait, it sets itself at the tick it knows it can run, and does not try again at every tick. An action is treated differently only when the owner proposes it.

Why: the effect system depends on every action going through the same pipeline and the same mechanisms, so that an effect can change any action.

## Behavior by kind is a table

Where code chooses behavior by a kind (a command type, an error, a constraint, a chat channel, a kind of change, a kind of bounce decision), use a table or registry of small units keyed by that kind, not a chain of conditionals or handlers that return whether they handled the message. A test fails when a kind has no entry. Examples: `route` by chat channel, `standing` by kind of effect, `decide` by kind of bounce decision, `infos` by action ID.

## No `Count` sentinels

A constant that stands for "how many values there are" (`ChannelCount`, `KindCount`, `GateCount`, `idEnd`) is not used. It sits among the real values of an enum, so it can be taken for one; it closes the list of values, against open for extension, above; and it breaks silently when a value is added after it.

Where a table or an array is sized by the values of an enum, use a table that is open to extension instead: a map keyed by the value, or an entry that declares itself. A list of values that has to be walked is kept where the values are defined.

When a count really is needed, ask the owner first. Then it is named so that it cannot be mistaken for a value, and it carries a comment that says plainly that it is a count and not a value that can be used.
