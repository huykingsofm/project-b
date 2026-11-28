# Rules

The rules of project-b. They apply to every phase of the [plan](../en/phases.md), which covers a whole online game and not only the simulation core. A rule marked **(hard)** in its file has no exceptions unless the owner decides one.

Each rule has a file of its own that says what it forbids or requires, why, and how it applies; this index gives a short summary of each. When a rule changes, change its file and its summary here, and every other place that mentions it, in the same change. A new convention goes into the rule it belongs to, and rules that say alike are merged: a new numbered rule is written only when none fits ([ways of working](ways-of-working.md)).

## The rules

### [R0. SOLID, above all open for extension (hard)](r00-solid.md)

Every design follows SOLID, above all open for extension: a new kind, use or provider is added by writing new code, never by changing code that works. No action is treated specially; behavior by kind is chosen by a table; no `Count` sentinels.

### [R1. Layers depend on each other only through interfaces the caller defines (hard)](r01-layer-dependency.md)

Layers never import each other. A layer reaches another through a small interface that the caller defines in its own package, with only the methods it uses; adapters implement them and only `cmd/*` wires them.

### [R2. Business stays in its own layer (hard)](r02-business-in-own-layer.md)

A layer never carries the business of another: inner layers offer mechanisms that know nothing of their users. Until the tag `base-v1` the base carries none of this game's business; sample content is allowed when marked as an example.

### [R3. Where shared code lives (hard)](r03-shared-code.md)

Use an external library when one exists; only what none provides is written in `shared`, as its own package that imports nothing of the project. `platform` is what any game that runs in ticks is made of, `server/game` is this game. Being common to several engines does not make something shared.

### [R4. What the design makes essential is built now, and its shape is made for what is coming (hard)](r04-essentials-built-now.md)

What the design makes essential is built now, with a test; only an opt-in optimization waits. A mechanism is shaped for all the uses the design and the plan foresee, not for the smallest set that the code needs today; when that is not clear, ask.

### [R5. Every pattern has at least one exemplar, and the owner decides the rest (hard)](r05-pattern-exemplar.md)

Every pattern has at least one exemplar, with tests, a benchmark and a short note. State methods, actions and the mechanisms behind them are the owner's to define: nothing is added on the author's initiative, but a design the owner approved is written without asking each time.

### [R6. Change the code: refactor at once, replace in place, look first (hard)](r06-refactor.md)

Refactor at once, in the same change or right after, as far as the better design needs. A replacement is made in place and the old is deleted, and the text reads as if it had always been so. Before writing anything, look for what already does it, and merge what does alike.

### [R7. Names and organization of files (hard)](r07-organization.md)

A file holds one responsibility. A name says the whole of what it is, told apart from its neighbours; one thing has one word and one word one thing; alike names follow one pattern. No aliases. State methods have explicit names. Settled terms: scope, zonelayer, channel.

### [R8. Player first, client first (hard)](r08-player-first.md)

The player's experience comes before security. What the player feels at once is done on the client first and checked by the server; the rest waits for the server. The engines never wait for each other.

### [R9. State is data, the events are a region of its own, and containers keep their order](r09-state-and-events.md)

A state is data and nothing else; only its engine changes it, through its methods. It has a data region, published as a `Clone`, and an events region. Containers are `stablemap`, `ring` and `table`, never a Go `map`, so the order is fixed; time is the tick and randomness the seeded RNG.

### [R10. Errors: actions do not fail, nothing panics, an error is returned or logged once (hard)](r10-errors.md)

An action returns no error. Nothing panics once the server runs. An error is returned, wrapped with where it went, or logged once by the one that handles it last, never both.

### [R11. Actions: names, files and IDs (hard)](r11-action-naming-and-files.md)

Every action belongs to one scope (the world, a zone or a zonelayer): its type, file and ID are named by that scope, the last word of its name is a verb, and the numbers of the register are never changed. A command is named, filed and numbered the same way.

### [R12. Every piece of data has exactly one owning engine (hard)](r12-one-owner-per-datum.md)

Every piece of data belongs to exactly one engine. Another engine reads a published `Clone` and asks for a change by a message. Moving data: remove first, then send; save after the tick ends.

### [R13. Every piece of data in the simulation is a value or can be cloned (hard)](r13-clone-or-value.md)

Every type held in a state is a value or has a `Clone` that copies everything; nothing is shared between a state, an engine and a published copy. What crosses between goroutines is a value; tests find any sharing.

### [R14. Documents of the code are reference, written as structure (hard)](r14-docs-are-reference.md)

Documents of the code are reference, written as structure: a fact is a list item, a key and value or a table row; one shape for one kind of thing; nothing the code or a rule already says; one home for each fact.

### [R15. A change to how data is encoded makes a new version, never overwrites the old (hard, after pre-alpha)](r15-versioned-data-formats.md)

A change to how data is encoded makes a new version and never overwrites the old (after pre-alpha). In pre-alpha every breaking change, the protocol included, is made as it is, with a wide refactor and no back-compatibility.

### [R16. A type with its methods right below it, the order of a file, and its lines (hard)](r16-methods-with-their-type.md)

A type and its constructors and methods are in one file, one after the other. A file goes: constants and variables, public functions, public types, private types, private functions. A type with no method is above the one that holds it, one with methods below. Fields and functions of one meaning are together. A line is at most 100 columns.

## Ways of working

### [Ways of working](ways-of-working.md)

Ask until every point is clear before starting; run `make ci` before every reply, commit and push; commits have a subject and a body wrapped at 72 columns, with no Claude attribution and no "part X"; delete merged branches; the plan is the source of work; run the benchmarks after a change that could affect them; use Go 1.27 fully; edit and merge the rules before writing a new one; replies are in Vietnamese with English technical terms, and everything in the repository is in English.
