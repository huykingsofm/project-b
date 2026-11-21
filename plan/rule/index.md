# Rules

The rules of project-b. Most come from the working conventions and architecture principles of the project-a roadmap, rewritten for project-b; section 4 holds the rules that are new here. A rule marked **(hard)** in its file has no exceptions unless the owner decides one.

Each rule has a file of its own that says what it forbids or requires, why, and how it applies; this index gives a short summary of each. When a rule changes, change its file and its summary here, and every other place that mentions it, in the same change.

## 0. Above all

### [R0. SOLID, above all open for extension (hard)](r00-solid.md)

Every design follows SOLID, and above all it is open for extension: a new kind, use or provider is added by writing new code, never by changing code that works, so what varies is given to a mechanism (a function, an interface, an entry that declares itself) and not branched on inside it. When R0 conflicts with another rule, the other is read in its light.

## 1. Architecture

### [R1. Layers depend on each other only through interfaces the caller defines (hard)](r01-layer-dependency.md)

Layers never import each other. A layer reaches another through a small interface that the caller defines in its own package, in its own vocabulary, with only the methods it uses; adapters implement those interfaces and only `cmd/*` wires everything together. A linter enforces it as soon as the project has one.

### [R2. Business stays in its own layer (hard)](r02-business-in-own-layer.md)

A layer never carries the business of another layer, in names, comments, options, queries or tests. Inner layers offer generic mechanisms that know nothing of their users; the layer that owns the business declares, fills in and interprets them.

### [R3. Where shared code lives (hard)](r03-shared-code.md)

Use an external library when one exists; only what none provides is written in `shared`, each as its own package that imports nothing of the project. Plan, scheduler and cron are game logic and live in `sim/action`, and `engine` runs the engines without knowing what the actions do.

### [R4. Generic base until the tag `base-v1` (hard)](r04-generic-base.md)

Until the tag `base-v1`, the mechanisms, names and config variables of the core carry none of this game's own business. Sample content may be added as marked examples that can be replaced without changing the mechanism. The business enters later through the extension points of the base, and a semantic review precedes the tag.

### [R5. What the design makes essential is built now (hard)](r05-essentials-built-now.md)

Whatever the design says every engine, message or level has is built now, with a test, and written into the design documents and the plan, never postponed until something uses it. Only opt-in optimization waits, and benchmarks still run.

### [R6. Every pattern has at least one exemplar](r06-pattern-exemplar.md)

Every pattern the design relies on has at least one working exemplar with tests, a benchmark and a short note on what using it requires, even when no feature uses it yet. The exemplar settles design and code before the first real use.

### [R7. No action is treated specially (hard)](r07-no-special-actions.md)

No action gets a field, a pattern, a part of the protocol or a code path of its own. A need that shows up with one action is met by a general mechanism (the gate) that every action uses with its own rule, and an action that must wait sets itself at the tick it knows it can run.

### [R8. Choose behavior by kind with a table](r08-dispatch-by-kind.md)

Where code chooses behavior by a kind (command, error, channel, change, decision), it uses a table keyed by that kind instead of a chain of conditionals, and a test fails when a kind has no entry.

### [R9. In memory first (pattern)](r09-in-memory-first.md)

A repository that is read often has an in-memory layer that is the source of truth, and the database is written afterwards by a bounded, ordered queue that is written out at shutdown. Writes that depend on the database stay synchronous.

## 2. Code

### [R10. Writing: replace in place, read as if always so](r10-writing.md)

A replacement is made in place and the old text is deleted: no old copies, shims or dead code. Edited text reads as if it had always been so, other mentions are updated in the same change, scans are semantic reviews, and a test goes with the thing it tests.

### [R11. Refactor as soon as possible (hard)](r11-refactor.md)

Refactor as soon as it is possible, in the same change or in a pull request right after. When a refactor gives a better design, do it; never work around a problem or put a thin layer over a design that should change.

### [R12. Organization of files and names (hard)](r12-organization.md)

A file holds one responsibility and a name says what a thing is and is not reused for something else. Split mixed files and rename vague or colliding names in the change that finds them. The queue an engine receives through is a channel, never a mailbox.

### [R13. Performance: benchmarks are mandatory](r13-benchmarks.md)

After a change that could affect performance or behavior, run the tests, the linter and the relevant benchmarks, compare with the previous numbers and record them in the PR. Opt-in optimizations are deferred, and a measured cost that is not a design problem is recorded and left alone.

### [R14. The protocol is not frozen until the first release](r14-protocol-not-frozen.md)

Until the first release the protocol is not frozen and wire compatibility is not checked. A breaking change is made whenever it makes the design simpler or sounder, without shims, and the owner decides when checking starts.

## 3. The player's experience

### [R15. Player first (hard)](r15-player-first.md)

The player's experience comes before security. No limit, prompt, expiry or step that bothers the player is added for a risk that is not realistic, and a security measure that costs the player something needs the owner's decision.

### [R16. Client first (hard)](r16-client-first.md)

For what needs low latency, the rule lives on the client, the server checks it and decides what is real, and the reply only confirms; everything else waits for the server, and a waiting client does nothing. The server is optimistic in the same way: engines never wait for each other and read the latest state another published.

## 4. Rules new in project-b

### [R17. The plan, the scheduler and the cron are not in `shared`](r17-plan-scheduler-cron.md)

Plan, scheduler and cron are game logic: they live in `sim/action`, never in `shared`, and a package of `shared` never imports anything of the project. Being common to several engines does not mean having no business.

### [R18. State is data, and the events are a region of its own](r18-state-and-events.md)

A state is data and nothing else, changed only through its own methods by the goroutine of its engine. It has two regions: the data, of which a clone is published at the end of the tick, and the events, the list of what to tell the clients, which is never cloned or published. Each state and its events are a package of their own.

### [R19. Actions do not fail and never leave their engine](r19-actions-do-not-fail.md)

An action returns no error and never leaves its engine: when it cannot do what it was asked it changes nothing or says so through the state. What must happen outside goes in the registry, and every send names what a failed delivery means.

### [R20. No Go `map` in state; iteration order is fixed](r20-no-go-map-in-state.md)

State never uses a plain Go `map`, because its iteration order is random and breaks a replay. Containers are visited in a fixed order, time is the tick count, randomness comes from the seeded RNG of the engine, and configuration from a snapshot taken once per tick.

### [R21. Go 1.27: use generics and the newer syntax when needed](r21-go-version-generics.md)

The project uses Go 1.27, so generics and the newer syntax are used when they make code clearer: a type-agnostic mechanism is generic instead of `any` with a type assertion, and nothing the standard library provides is rewritten by hand. A change of Go version is the owner's decision.

### [R22. Design for what is coming, not only for what is used today (hard)](r22-design-for-the-future.md)

A mechanism is designed for the whole set of uses that the design and the plan foresee, not for the smallest set that today's code needs, so that the next kind or use is added and nothing that exists has to change. Only the feature waits, never the shape of the mechanism, and when the foreseeable uses are not clear, ask before fixing the shape.

### [R23. No `Count` sentinels (hard)](r23-no-count-sentinels.md)

A constant that stands for "how many values there are" is never used: it can be mistaken for a value, it closes the list of values, and it breaks when a value is added after it. Use a table that is open to extension, and when a count is truly needed, ask the owner and comment that it is a count and not a usable value.

### [R24. No aliases (hard)](r24-no-aliases.md)

A name is never defined as another name for something already defined, not even a generic alias, a `var` or a `func` that only renames. The origin must be found at once from where a thing is used, so the real name is written even when it is long.

### [R25. Actions: names, files and IDs (hard)](r25-action-naming-and-files.md)

Every action belongs to exactly one scope (world, zone or zonelayer), all in one package. Its type is the scope, then the words of its name, then `Action`, its file is `<scope>_<name>_action.go`, the words go from the general to the specific so related actions stand next to each other, it implements the interface of its scope, and its ID is a `struct{}` type defined in its own file. A cancellable action embeds its scope's action interface.

### [R26. The owner decides state methods and actions (hard)](r26-owner-decides-state-and-actions.md)

A state method, an action and the mechanism of either are the owner's to define. None is added on the author's own initiative; when something is needed, say what and ask how it should work. What exists stays as the example of its pattern, and the owner reviews each one.

### [R27. State methods have explicit names](r27-explicit-state-method-names.md)

The name of a state method says plainly what it does and to what, even when long: `SendMessage`, `AttachCharacter`, `MoveEntity`, `DamageEntity`.

### [R28. Every piece of data has exactly one owning engine (hard)](r28-one-owner-per-datum.md)

A piece of data belongs to exactly one engine and only that engine changes it; others read the published copy and ask for a change by message. When data moves between engines it is removed first and sent after, and it is saved only after a tick has ended, so a failure can lose data but never duplicate it.

### [R29. Every piece of data in the simulation is a value or can be cloned (hard)](r29-clone-or-value.md)

A type held in a state is either a value type (no pointer, slice, map, channel or function) or has a `Clone` method that the `Clone` of the state calls. An interface held in a state is implemented only by value types, and a change replaces the value. Nothing is shared with a published copy.

## 5. Ways of working

### [Ways of working](ways-of-working.md)

Ask until every point is clear before starting, and state what only one option can do when asking for a decision; run `make ci` before every reply, commit and push and never skip it; every commit has a subject and a body wrapped at 72 columns, with no Claude attribution and no "part X" labels; delete merged branches; read the plan at the start of each session and record pending work in it; replies are in Vietnamese with English technical terms, and everything in the repository is in English.
