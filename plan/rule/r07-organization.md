# R7. Names and organization of files (hard)

A file holds one responsibility and its name says what that is. A type, function, constant or package name says what the thing is, and is not reused for something else in the repo. When a file mixes responsibilities (public types, the state of one thing, the behavior of another), split it. When a name is vague, wrong, or collides with another (a type named like a package, one word used for two things), rename it. Do it in the change that finds it, or in a pull request right after. A scan for such files and names is a semantic review of every package, not a keyword search.

A name says the whole of what it is about, so that it is told apart from every other name near it and read alone without its comment. A name that would fit two things in the code names the one it is: on a cluster that serves requests of two kinds, `serveRequests` is `serveDirectiveRequests`. One thing has one word (an engine is retired, not "removed" in one name and "retired" in another), and one word means one thing (an `ack` of a message is not the acknowledgement of a response). Names that are alike (two lists that wait for something) follow one pattern (`pendingCreations`, `pendingRetirements`).

Settled terms: the world, a zone and a zonelayer are the three **scopes** of this game: a word of the game (in the names of its actions, R11), not a type, and the platform has no such concept: it knows states, each of a type (`StateType`) and with an ID of its own (`StateID`). `level` is the level of a character and is not used for them. The layer of a zone is called a **zonelayer** and never just a layer, because a map has layers too (a layer of tiles) and the two are easy to confuse. the queue that other code sends an engine things through is a **channel** (a Go channel), never "mailbox"; a zone stays a zone.

## No aliases

A name is never defined as another name for something already defined: not `type ZoneLayerAction = sim.Action[*zonelayerstate.State]`, not a generic alias such as `type ZoneLayerPlan = sim.Plan[*zonelayerstate.State]`, not a `var` or a `func` that only renames another one. The place where a thing is defined must be found at once from where it is used; a short alias is never worth a lookup.

Write the real name where it is used, even when it is long. Use `sim.Action[*zonelayerstate.State]` and `*sim.Plan[*zonelayerstate.State]` directly.

A defined type that is really a new type, such as `type ID interface{}`, is not an alias. An import alias (`zonelayerstate "…/state/zonelayer"`) only picks a local name for a package and is allowed.

## State method names

The name of a method of a state says plainly what it does and to what, even when that makes it long. It does not have to read well, it has to be unambiguous: `SendMessage`, not `Say`; `AddCharacterEntity`, not `Attach`; `MoveEntity`, not `Step`; `DamageEntity`, not `Hurt`.

A state method that reads or writes something of one kind of thing names that thing: `CharacterByID`, `CooldownUntil`, `SendPrivateMessage`.
