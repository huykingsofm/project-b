# Phases

Every phase has tasks and a measurable acceptance. There are no weeks: a phase starts when the one before it has passed acceptance. Every design marked `(pending approval)` needs the owner's approval before any code is written; state methods and actions are the owner's (R26). Running phases in parallel is up to the owner, who opens separate sessions and branches; the plan only states the dependencies.

The performance targets are the minimum for every phase: 2,000 players at once on one machine (one machine is one world), p99 of a tick under 20 ms at the network layer and under 45 ms per layer.

**Stage: pre-alpha.** There is no official production yet, so any breaking change in the code (not only a data format) is made as it is and everything that depends on it is refactored, with no version and no back-compatibility ([R32](../rule/r32-versioned-data-formats.md)). It lasts until the owner says it is over and changes this line; from then on R32 applies in full.

## Progress

Every phase records its progress directly under its heading, in a `**Progress:**` line with one of three values: `not started`, `in progress` or `done`. A short note after the value says what is already built and what is missing. The line is updated in the same PR that lands a task of the phase, and a phase becomes `done` only when its acceptance has passed.

## Order and dependencies

```
0 Foundation: entities and items
  1 Durable engine: checkpoint and replay log
    2 Early playable slice (first client: fight, pick up, drop, monsters)
      ├─ 3 Space ── 4 Characters and combat ── 5 Effects
      ├─ 6 Storage
      ├─ 7 Network and accounts (complete)
      └─ 8 Client and tools (complete)
            9 Content before base-v1 ── tag base-v1
                                          10 Game business
```

After phase 2, the branches 3 to 5, 6, 7 and 8 do not depend on one another beyond the interfaces already fixed; phase 9 needs all of them.

---

## Phase 0: Foundation: entities and items

**Progress:** done.

Clean up what is unfinished and lay the foundation for entities before building further.

- Update `README.md`, `docs/action/*.md`, `docs/entity/*.md` and the rules (R3, R4 and the opening of `plan/rule/index.md`) to match the scope of the plan.
- **An entity is an interface** (detail pending approval): `ID()`, `KindID()`, `Position()`, `Radius()`, `BoundingBox()` (the smallest rectangle that contains the entity, derived from the position and the radius, for the grid). Collision uses the radius. Every entity is a value type (R29); a change replaces the value in its container.
- `CharacterEntity` and `ItemEntity` (a dropped item, removed when its lifetime ends), and `ExitEntity` (an exit of the map, which the map gives to every zonelayer of its zone as an entity). The body of a character is moved to this interface.
- A pickup action: the picker must be within a given radius and chooses the exact `EntityID` of the item (the client works out which). The item is removed from the zonelayer first, and only then is a message sent to the world to add it to the inventory (R28). (pending approval)
- Events: item appeared, item disappeared (with an ID registered in `events.go`).
- An R29 test for every state: changing the state after `Clone` does not change the copy.
- Rules R28 (one engine owns a piece of data) and R29 (value or `Clone`) are written; keep them in step with `plan/rule/index.md`.
- Business examples in the code (item, monster, sample stats) are only examples, to expose a design that does not fit early; a base game changes them later.

**Acceptance:** `make ci` passes; the documents match the code; pickup tests (inside and outside the radius, an item already taken, an item removed at the end of its lifetime, two players at once and only one gets it); the R29 test; the owner has confirmed every "pending approval" point of this phase.

## Phase 1: Durable engine: checkpoint and replay log

**Progress:** in progress. The `replay.Log` writes to files for each engine (`replay.FileLog`: the records of every kind, commands with an ID and a codec, the action IDs, command IDs and cooldown keys as numbers of their registers, on `shared/segmentlog` with its repair after a crash and group commit) and can be read back from a tick, and it holds the same records as the memory log in a whole-cluster run. `StableView` returns the read-only interfaces. Checkpoints, the scheduler in a checkpoint, the replay runner, the cooldown conversions and the other tasks below are not built.

- **The replay log is written to a file, append-only** (like Kafka): divided into segments, indexed by `Step`; not pushed to S3 yet. The format is agreed, see [proposed-designs.md](proposed-designs.md). Built: [replay_log](../../docs/replay_log.md).
- **Benchmarks on a fixed number of cores (pending approval), done early in this phase, before the checkpoint:** the server will run on a machine with one or two cores, the simulation included, so the benchmarks of the engine, the simulation and the replay log are run with the cores limited to 1 and to 2 (`GOMAXPROCS` and `taskset`) and not left to use all of them. The results in `docs/benchmark/` get a column or a table for each, and the many engines of `FileTickManyEngines` are measured paced, as an engine ticks, with the p99 and the worst tick, besides the saturated run.
- **Periodic checkpoint:** write the whole state of some engines so that a replay does not have to start from the beginning. The period is a config parameter, much rarer than the hash. Engines do not write at the same time: each engine has its own offset (like the sparse hash) to avoid CPU peaks. Writing happens after the tick ends (R28). (agreed)
- **The checkpoint holds what the engine is waiting for, and not only the state:** the scheduler (what is set for a tick), the cron (when each runs next) and what the actions set for the next tick. Without them a restore loses every action that waits (a shield, a run of movement, the removal of an item), and the state alone shows no sign of it. Each action has a codec, found by its ID in a table (R8) with a test that fails when an action has none; the hash covers the scheduler too, so a restore that differs is found by the replay. (agreed)
- **Every place that puts an item on the ground goes through `ZoneLayerItemEntityAddAction`,** the only place that sets the removal of an item, because a state method cannot schedule anything. (agreed)
- **The log and the checkpoint belong to the cluster (pending approval; the design is in [proposed-designs.md](proposed-designs.md)):** each engine writes its own files and stays independent, and the files are one log of the cluster with a manifest. The checkpoint of the cluster is a set of one snapshot for each engine. Loading is the replay: no engine loads or replays alone.
- The replay runner reads the whole log, runs every engine again in an order that respects what depends on what (messages, copies read, messages that came back, engines added or removed; there is no global tick) and compares the hash at each recorded point. It starts from tick 0 or from a set of checkpoints. It runs the real engines with replay adapters in place of the router, the store, the emitter and the log, so going live after a restart is changing the adapters back.
- **Replay in parallel, as live does:** after the runner that plays the engines in turn, one goroutine for each engine, each waiting for the messages, copies and returned messages it depends on, so a replay uses the cores the cluster uses. (a later optimization)
- **Nothing of the log or the checkpoints is deleted:** deleting, archiving and storage of old segments and snapshots are a separate job, planned later.
- Add and remove zonelayers while running; a zone creates a layer when someone arrives (on demand). (pending approval)
- Change layer within the same zone (a person moves to another layer). (pending approval)
- **`StableView` returns read-only interfaces (built):** `World()`, `Zone(id)` and `ZoneLayer(id)` return `ReadOnlyWorldState`, `ReadOnlyZoneState` and `ReadOnlyZoneLayerState` (the read methods of each state, none that changes it), not the state itself. An action then cannot change a state it does not own by mistake (R26, R28), and the compiler checks it instead of a review. The read-only interface of a scope is declared with its state.
- Graceful stop: `Run` ends, drains the queue and returns the messages that remain.
- Back-pressure: count the messages that are refused (`Full`) and lost (`Dropped`), with configured thresholds.
- Plain engine metrics: players, capacity, tick time histogram (the engine does not know Prometheus). (pending approval)
- A test that `Step` and `Run` give the same result for the same input.
- A baseline benchmark for one full zonelayer and for many zonelayers in parallel.
- **A benchmark for every mechanism and action that is built, recorded in `docs/benchmark/`** (a file for each group, always the latest numbers; the path of a move is the first, in `docs/benchmark/move.md`): the plan and the scheduler, each action, the state methods, the hash and the clone of each state, an engine tick with and without a crowd, the router and the replay log.

- **Cooldowns follow a character from one engine to another and through the store (agreed, detail pending approval).** A cooldown is the first tick at which a character is free, in the clock of the engine it is in (`Cooldowns`). The engines tick at their own pace (`Cluster.Run`), so that number means nothing in another engine, and a character that goes through an exit or comes back from the store would gain or lose cooldown. The cooldown has three forms, each in its own layer: an instant of real time (UTC) for each key, in the store and the adapter around it; the number of ticks still to go, on a `Character` while it moves (in a message or a command); and the tick it ends at, in the state of a zonelayer. Two types keep the last two apart. The action that takes a character out of a zonelayer turns each `until` into `until - tick` and drops the keys that are done; the action that brings it in turns that into `tick + remaining` with the tick of the engine that takes it in, so the engines use nothing but ticks and a replay stays exact. The adapter between the store and the engines does the part that needs the clock: when it saves, the instant is now plus remaining ticks times the tick period; when it loads, the remaining ticks are the time left until that instant divided by the tick period, rounded up, and they go in the command. A cooldown keeps running while the character is offline, because the store keeps the instant. Rounding up means a cooldown never ends early, and the error is at most one tick; a message through an exit adds the time it waits in the inbox of its receiver (one or two ticks). The replay log records the keys and the numbers of a command as they are carried, so it does not depend on this. The saving and loading are in phases 6 and 7.

**Acceptance:** a replay of the whole cluster from a checkpoint matches the hash, with actions waiting in the scheduler at the moment of the checkpoint (an item with an expiry, a shield, a run of movement that waits its turn); the checkpoints of the engines do not fall on the same tick (CPU peak measured); the metrics and benchmarks are recorded; a kill and a graceful stop do not lose a message that was received; a character that goes to an engine whose tick is different keeps its cooldowns to within the ticks its message waited. (pending approval)

## Phase 2: Early playable slice

**Progress:** not started

Goal: a client that works from end to end (client to server and back to client) with the basic functions, so that the later phases can run in parallel with the frontend.

- **Protocol:** protobuf with `buf`, binary frames over WebSocket, events batched per tick, generated code for Go and TypeScript. (pending approval)
- **Gateway:** WebSocket; authentication once when the connection opens (password hashing with argon2id, session token), later messages are not authenticated again; heartbeat; a bounded send queue, slow clients are disconnected. (pending approval)
- **In-memory store** (accounts, characters) behind a port defined by the caller (R1); Postgres takes its place in phase 6 without changing the simulation.
- **Functions:** enter the world, walk, fight (normal attack), monsters (NPCs with a minimal AI), a drop when a monster dies, pick up an item, chat.
- **Web client:** TypeScript, PixiJS and Vite; it only needs to draw the map, entities, health bars, a minimal inventory and chat. The owner does not review the frontend, so the tools are chosen to be fast and stable, and the code is kept small.
- A Go bot that uses the same protocol to test.

**Acceptance:** a real player enters from a browser, walks, fights a monster, sees the drop and picks it up; 2,000 bots enter and walk at once, p99 of a tick under 20 ms at the network layer and under 45 ms per layer; a client that does not read is disconnected while the server keeps running; a packet test shows no information leaks.

## Phase 3: Space

**Progress:** not started

**Priority:** high; planned for the near future, before the later phases that depend on telling clients what changed. The choices behind it are in [decisions.md](decisions.md).

- **Grid:** a spatial index and localizing the map: each entity belongs to one or more cells by its `BoundingBox`, lookup by area, and knowing which cells changed in the tick. (agreed, detailed design pending approval)
- **Sight:** entering and leaving sight; who sees whom. (pending approval)
- **Loading state:** a new entity stays hidden until it has loaded. (agreed, detail pending approval)
- **`observe`:** project events into observations per viewer; group by cell once and reuse; a no-leak test (see [proposed-designs.md](proposed-designs.md)). (pending approval)
- **Resolving audiences (high priority):** every event already declares who receives it (`Audience()`: everyone in the scope, a list of characters, a list of players, or whoever can see an area). This turns an audience into the characters that receive it, and then into player connections or NPCs. An NPC receives events like a player does and decides from them.
- **Delivery to clients (high priority):** an event goes to channels, and the response of a channel is built once and then sent to everyone who listens to it. A public event (the world, a zone or a zonelayer) is built once for the whole scope and broadcast to every connection in it; an event for an area is built once per grid block and sent to the viewers of that block; only an event that is private to some characters or players, or to a group (a sect, a sect inside a zone), is built for those recipients alone. The event declares its audience and nothing else; the code that routes and sends is generic. The design is in [decisions.md](decisions.md).
- **One event or two (high priority):** when different viewers must see different things of the same fact, emit two events with two audiences ("you picked up 3 swords" private to the owner, "someone picked something up" for the area), so every response stays shared and a secret never reaches the wrong channel.
- **The cost of telling clients (high priority):** with N entities that move and V viewers in the same crowd, delivering each move to each viewer costs N times V every tick (22,500 deliveries for 150 and 150 in one layer). Building a response once per channel is the main saving; the other ways to reduce it (one move per entity per tick, a lower send rate for what is far, a cap of visible entities, known-sets of who sees whom) are listed in [decisions.md](decisions.md), with a benchmark of a crowded layer to choose by.
- **A body is blocked by other bodies, for some of them and not for others (agreed, detail pending approval):** a move already stops at tiles that cannot be stood on (the body is a square of its radius, of any size). Bodies of entities can stop each other as well, and not every body does. Each body says whether it blocks and which others block it, so that, for example, some bosses cannot be walked through while players walk through each other (and through monsters that do not block). The rule is a table over the kinds and groups of body (R8), not a branch in the move; the lookup of the bodies in the way uses the grid, so it costs what is near the path and not the entities of the layer; and the move is benchmarked with a crowd (`docs/benchmark/move.md`). **Where each collision is computed:** terrain stays in `TileMap` (one bit for each tile; it never changes and every copy of the zone state shares it), and collision with other bodies, including things smaller than a tile, is computed in the zonelayer from the entities found through the grid, never in `TileMap`. A move is two steps: `TileMap.Reach` gives where terrain stops the body, then the bodies near that stretch of path can stop it earlier; sliding along a body is not planned. (pending approval)
- Read the map from a file (Tiled, or another format the owner picks): map, spawns, destinations, exits. (pending approval)
- More kinds of entity when needed: corpse, resource, portal, trap, projectile (see [proposed-designs.md](proposed-designs.md)). (pending approval)

**Acceptance:** tests for entering and leaving sight; the no-leak test; lookup by area is correct; a benchmark of 150 entities in one layer is recorded.

## Phase 4: Characters and core combat

**Progress:** not started

- Character stats and derived stats come from a data file; table-driven tests. The set of stats is decided by the data. (pending approval)
- Attack and defense formulas (replacing the temporary damage `1`), dodge, critical hit. (pending approval)
- Death, respawn (a gameplay design), level and experience. (pending approval)
- Skills (normal attack is a skill) with the cooldown as a config parameter. (pending approval)
- A design for hit resolution under network lag: how far the server trusts the client. (pending approval)
- Replace the temporary regen with the owner's new design. (pending approval)

**Acceptance:** formula tests by table; death and respawn are correct; the replay of a fight keeps the hash.

## Phase 5: Effects

**Progress:** not started

- The effect framework: the `Effect` interface, a registry by ID, standing actions by scope (the owner acts, the owner is acted on, around the owner, in the zone, on the whole server). (pending approval)
- Plan indexes by kind and target when a benchmark requires it (`FindByID` and `Find` exist).
- 10 sample abilities covering all three scopes; an activated ability stays in the state until it expires. (pending approval)
- Ability secrecy: recognized through `observe`, with a no-leak test.
- **Performance gate:** one full layer of 150 entities and 4 layers in parallel, each entity with one effect ability, p99 of a tick under 45 ms per layer. If it is not met, stop and fix the effect design before building further. (agreed)

**Acceptance:** the performance gate is passed; the no-leak test through `observe` passes; every sample effect has a test and lives in one file.

## Phase 6: Storage

**Progress:** not started

- **Postgres** holds the important data: accounts, characters, inventory, configuration, chat history, seasons. The schema is divided by account and by season; migrations (goose); generated queries (sqlc). (pending approval)
- Replace the in-memory store of phase 2 with a Postgres adapter, without changing the simulation.
- A write-behind queue: save positions and characters without blocking a tick; always write after the tick ends (R28). (pending approval)
- Character names: normalization, alphabet, duplicate check, banned words. (pending approval)
- Push closed replay log segments and checkpoints to S3, keeping the keys and hashes in Postgres. (low priority, last in this phase)
- A lock on the directory of a replay log (in `segmentlog.Open`: flock or a lock file), for when more than one process can run: a restart that overlaps the old process, or several servers on one disk. Until then one process owns a directory by convention, and two writers would corrupt it. (pending approval)
- Seasons and offline cultivation computed once at login (a gameplay design). (pending approval)
- Preventing duplicated items: an item has an ID of its own and exactly one owner (an entity on the ground, an inventory or a corpse); a transfer is one operation (R28); entity and item IDs are never reused. (pending approval)
- Cooldowns: saved as an instant of real time for each key and loaded as the ticks that are left, by the adapter between the store and the engines (see phase 1). (pending approval)
- `Character` already moves intact between layers: reuse it for saving and loading.

**Acceptance:** after a sudden stop and a restart, a character keeps its position and inventory (error at most one write period) and no item is duplicated; tests on a real Postgres with `-race`.

## Phase 7: Network and accounts (complete)

**Progress:** not started

The basics of the gateway and the protocol exist from phase 2; this phase completes them.

- The full protocol: error codes; text keys and parameters for localization; a protocol version. (pending approval)
- Gateway: a command rate limit, packets batched per tick, reconnection. (pending approval)
- Accounts: guest, registration, linking a password; one connection per account. (pending approval)
- Character creation; chat history.
- Configuration in the database, reloaded while running (a watcher), a whole set is rejected when one value is wrong.
- Adapters follow R1; `cmd/*` is the only place that connects the layers.

**Acceptance:** 2,000 bots log in, walk across many zones and chat; p99 of a tick under 20 ms; the packet no-leak test passes; the command rate limit tests pass.

## Phase 8: Client and tools (complete)

**Progress:** not started. The git hook that runs `make ci` before a commit exists (`make hooks` installs it).

- Web client (TypeScript, PixiJS): the image pipeline, pixel art and map tools, the remaining screens.
- Load bots for measurement scenarios.
- Deployment: docker compose, CI, a git hook that runs everything CI runs.
- Observability (low priority): metrics, pprof, a dashboard, non-blocking logs; done last when the rest is stable.

**Acceptance:** a real player can play the full version in a browser; it runs with docker compose.

## Phase 9: Content before `base-v1`

**Progress:** not started

- Monsters with a full AI state machine, spawn areas, drop tables. The choices to make first are listed in [decisions.md](decisions.md). (pending approval)
- Items, inventory, equipment.
- Basic quests. (pending approval)
- A semantic review (R4): code, names, config variables, and sample content marked as examples; then tag `base-v1`.

**Acceptance:** the semantic review passes; tag `base-v1`; a player can fight a monster, pick up items and take a quest.

## Phase 10: Game business (after `base-v1`)

**Progress:** not started

The game's own business enters through the extension points of the base (R4): cultivation, quests and story, economy (a system market), sects, war and territory, large-scale abilities, seasons and closed testing, full localization. Each group will have a plan file of its own when its turn comes.
