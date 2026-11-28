# Proposed designs

Each item is an idea; the design for project-b is made again under the rules of project-b, and the owner approves it before any code is written.

## What `observe` and `effects` are

Both are only ideas so far, with no code.

**`observe`**
- An event is what happened in a state (for example `EntityMoved`). `observe` turns an event into what **each viewer** may see (an observation), and only then into a message sent out.
- It filters by sight (who is near, who is in view) and hides secret information: for example it never lets the ID or the name of an ability into a packet.
- The idea: only one package may build the messages sent out, so leaking an ability is blocked by structure, and a test scans the packets to make sure nothing leaks.
- In project-b: the `Emitter`s already send events out; `observe` will sit between the events and the packets.

**`effects`**
- An effect is an ability, a technique, a piece of equipment or a talisman. Each effect is a Go type that implements the `Effect` interface, lives in one file and registers itself by ID, with its numbers as config variables (power can be tuned without a deploy).
- An effect consists of **standing actions** that wait in the plan, watch the actions that stand after them, and change or cancel them, by scope: the owner acts, the owner is acted on, around the owner, in the zone, on the whole server.
- `ZoneLayerCharacterShieldGrantAction` in project-b is the first example of this kind (it blocks the first damage). The plan mechanism (`FindByID`, `Cancel`, depth) is its foundation.

## Entities

An entity is an interface with `ID()`, `KindID()`, `Position()`, `Radius()` and `BoundingBox()`. Collision and pickup distance use the radius; `BoundingBox()` is derived from the position and the radius so that the grid knows which cells an entity is in. Every kind is a value type (R13), and every kind has its own table in the zonelayer state, keyed by `EntityID`, with its own events.

| Kind | Example data | Phase |
|---|---|---|
| Character (`CharacterEntity`) | `CharacterID`, stats, health, shield, cooldowns | 0 (the body exists) |
| Dropped item (`ItemEntity`) | item type ID, quantity, who may pick it up first (removed when its lifetime ends) | 0 |
| Corpse | the `CharacterID` that died, a list of items, expiry tick, whether it can be revived | 3 |
| Resource | type, amount left, refill tick, tool required | 3 |
| Portal | destination zone and spawn point, level requirement, enabled or not | 3 |
| Trap | owner, trigger radius, effect ID, ready tick, uses left, who can see it | 3 |
| Projectile | owner, direction and speed, damage, range left or expiry tick | 3 |

The first two kinds are built in phase 0; the others are added when needed, and each is pending approval. An action says which kinds it applies to through the general mechanism (R0), with no special case for each action.

An item has an ID of its own and exactly one owner (an entity on the ground, an inventory or a corpse). Inventories belong to the world, combat stats to the zonelayer (R12). Picking up: the zonelayer removes the item first and then sends a message to the world; if the message is lost, the item is lost but never duplicated, and the log can restore it.

## Event log and checkpoint

Agreed (phase 1).

- **One writer per engine, one log for the cluster.** Each engine writes only to its own files, with no shared lock; the files are one log of the cluster, read as one when replayed (see the end of this section).
- **Two layers.** `shared/segmentlog` is a general append-only log: it knows segments, framing and the crash repair, and nothing of ticks or engines. The event log puts what it needs in the two places the segment log leaves to its user, the meta of a segment header and the payload of a record.
- **Segments.** `<dir>/<name>-<id>/seg-<index>.pbsl` (name of the type of the state, ID of the state; for example `zonelayer-4/seg-00000000000000000000.pbsl`), the index in 20 digits counted from 0 in the order the segments were made, closed when it reaches a configured size.
- **Record framing like Kafka's.** `[u32 length][u32 crc][payload]`, and a batch (a tick) ends with a commit marker, so a batch is all or nothing. After a crash, a half-written record or a batch with no commit at the end is detected through the length, the crc and the commit, and cut off (and kept next to the segment). The payload is the event log's own: `[kind][tick][body]`, with the tick whole in each record so that a record does not depend on its segment.
- **Segment header.** Magic, format version and a meta. The event log's meta is the format version of its records, the engine address, the seed, and the tick of the first record of the segment; a tick is found by reading the meta of about log2(n) segments, so no index file is needed.
- **Record bodies.** Commands and actions are written as numbers of registers (action ID, command ID, cooldown key); a command writes its own fields with its `Encode`, and a function in its file reads them, found by the ID in a table, so a new kind is new code and changes nothing that exists.
- **No cost during the tick.** The methods of `eventlog.Log` only append to a reused per-tick buffer. When the tick ends, the buffer goes to one writer goroutine that writes each batch with one `Write` and syncs at a configured interval (group commit), not on every tick.
- **Checkpoint.** One file `ckpt-<tick>.snap` for each engine (written to a temporary file, synced, then renamed), with no set for the cluster: each engine is on its own. The period is a config parameter, rarer than the hash, and offset per engine by the hash of its ID. What it holds and how it is taken: see the end of this section.
- **Recovery.** Of the cluster, not of an engine: it is the replay runner playing the logs from the checkpoints of the engines. See the end of this section.
- **S3.** Pushing closed segments and checkpoints to S3 is in the plan at low priority (phase 6), not now.

### The log and the checkpoint belong to the cluster (pending approval; replaces recovery by option A)

Principle:

- **The unit of recording and of recovery is the cluster.** An engine stays independent while it runs: its own goroutine, its own files, no shared lock. The log and the checkpoints are one thing for the whole cluster: one directory, one manifest, read as one by the replay. No engine loads or replays alone.
- **There is no global tick.** Engines tick at their own pace, so replaying the cluster means running every engine in an order that respects what depends on what, not in tick order.

What crosses from one engine to another (the only thing a replay needs besides the input of each engine):

| Crossing | Recorded today | Needed |
|---|---|---|
| A message (sender, `Seq`) taken by an engine at its tick | header and action IDs, at the receiver | the actions too (codecs by ID) |
| A copy of another engine read in a tick (`StableRead`) | the tick of the copy | nothing more |
| A message that came back (`DeliveryFailureFull`, `DeliveryFailureRetired`) and was settled at a tick | nothing | a record: the tick, the header of the message and the reason |
| An engine added or removed while running | not built yet | a record of the cluster: which engine, when it starts, and the engine and tick that caused it |
| The version of the configuration | at the first read of each version | the content of each version, in the control stream |

Why it can be done:

- Every crossing happened before what depends on it in real time, so the order is a graph with no cycle.
- An engine's tick may run once every message it took at that tick is known and every copy it reads exists. The runner runs an engine until it has to wait for another, and keeps each published copy until its last recorded reader has passed.
- The log decides who receives what at which tick, and the senders are replayed too, so the hashes at every recorded point are compared for every engine.

The log:

- `<dir>/manifest`: the cluster, its seed, the format versions, and each engine with its address, kind and the tick it was born at.
- `<dir>/<name>-<id>/`: the segments of each engine, as agreed; one writer for each file.
- `<dir>/control/`: the records of the cluster itself: engines added and removed, and the content of each version of the configuration (what `Store.Apply` was given), so that a replay needs nothing outside the log; the cluster writes it, not an engine.
- The manifest and the control stream are written by the cluster, never from a tick.

The checkpoint:

- **One file for each engine, at the tick of its own offset** (the hash of its ID, as the hash of the log has), so the engines do not write at the same time. What is written, in a file: the tick; the state data, by the codec of its scope; the scheduler (the actions set for each tick, in order); the self list; the state of the random generator; the sequence number of its messages and the last number it took from each sender; the version of the configuration read. Not the map of a zone (its ID finds it). What an engine does again and again is an action that sets itself again, so it is in the scheduler and nothing else is written for it.
- **Taken at the end of the tick, written outside the goroutine.** At its checkpoint tick the engine makes a `Checkpoint`: the clone it publishes as the state, and a clone of what is outside the state (the scheduler with each action, the random numbers, what its peer knows with each unsettled message), so it shares nothing with the engine; it hands its `Encode` to one writer goroutine for the cluster; the write is a temporary file, a sync and a rename. A writer whose queue is full drops the checkpoint and logs it. The log of the engine has the hash at the checkpoint tick, and no marker.
- **Staggered ticks are not a consistent cut,** so the replay chooses where each engine starts: the start tick of an engine is its latest snapshot not later than the smallest copy of it that another engine reads after that engine's own start, and the choice is repeated until it stops changing. It ends, at worst at tick 0. A start that goes far back is accepted: exactness comes before the time a replay takes, and how far each start went is a metric. A message in flight at the cut does not matter: its receiver has it in its log, with its actions.
- **Nothing is deleted.** Segments and snapshots stay where they are; storage and archive are a separate job, planned later.
- **Loading is the replay:** after a stop, the cluster starts as the replay runner does, plays every log to its end in dependency order, and goes live with each engine at its own tick.

Codecs and the hash:

- Every action gets `Encode(w)` and a decode function found by its ID in a table (R0); a test fails when an action of the register has none. The states of the three scopes get an `Encode` and a `Decode`, and each kind of entity is found by its `KindID` in a table.
- The hash covers the scheduler, by hashing the encoding of its actions.
- `Inbound` carries the actions, and the checkpoint carries the messages the bouncer holds.
- **`OnFail` is data, not a function: an interface, and each kind is a type of its own, in package `platform/sim/onfail`.** A kind holds what it needs (`onfail.Compensate{With}` the message that undoes what the first one set going, with its own `OnFail`; `onfail.Redirect{To}` an address), runs itself on the engine that sent the message through an interface that package `platform/sim` defines (`sim.Bouncer`), and writes itself (`Encode`); its decoder is found by its ID in a table. A new kind is a new type and a new ID. The vocabulary that the kinds and the actions share (`Header`, `Reason`, `Message`, `OnFail`, `Bouncer`) is in `platform/sim`, which the kinds import, so that the actions can build kinds and the kinds can name messages. The chain of compensations ends at a kind that holds no message (`Drop`, `Retry`). A message is written whole by one codec (the type of state, the destination, the actions, the `OnFail`), used for `Inbound`, for the checkpoint and for the message that compensates. Nothing a message does on failure depends on the `Reason` today; a kind that must can read it, since `Handle` is given it.

Messages in flight when the server stops (the reason a transfer of an item between two players must never lose or duplicate it):

- **Not lost:** a message that a sender replays but that no receiver recorded is sent again when the cluster goes live, and the receiver drops a second copy of the same `(From, Seq)`.
- **Not duplicated:** the logs of two engines reach the disk at different times, so a receiver may have recorded a message whose cause the sender did not. The recovery line cuts the log of the receiver at the first record that the sender, replayed to the end of its durable log, does not produce; the receiver loses what follows and nothing is duplicated. The runner finds it as it plays the engines in order.
- **What the log cannot give:** the last interval before a stop (`SyncEvery`) may be lost. What a player must not lose (an item) is saved in the store of phase 6, where an item has exactly one owner and a transfer is one transaction.

A message that came back (`DeliveryFailureFull`, `DeliveryFailureRetired`) depends on how busy the receiver was, so it cannot be worked out again: the log records the tick, the header of the message and the reason, and the replay of the sender settles it at that tick.

Replay is the engines running again, not a second implementation:

- The code of a tick (`Step`) is the same. What differs is where its inputs come from and where its outputs go, which are the adapters the engine already receives (R1):

| Seam | Live | Replay |
|---|---|---|
| ticks | a ticker | the runner calls `Step` |
| commands | the queue of the engine | the records of its log |
| messages in | the router | what the log says it took at each tick (the senders, replayed, produce them) |
| messages out and returned | the router | caught by the replay and delivered as the receivers recorded; returned as the log says |
| copies of other engines | the latest published | the copy of the tick recorded |
| configuration | the store | the content of the version recorded, from the control stream |
| events to clients and the event log | the emitter and the file log | nothing |

- **Going live is making the engines again in the real environment (agreed):** after the log is played to its end, each replayed engine is checkpointed at its last tick, and an engine is restored from that checkpoint (`Restore...Engine`) in the environment with the real router, store, emitter and file log; it goes on from its own tick. A replayed engine never runs live, so no engine that runs carries an adapter of the replay and the path of a running engine has no extra indirection. Recovery and replay share the runner; they differ in what is made at the end.
- What making them again needs:
  - what repeats needs no check: it is an action that sets itself again, so it waits in the scheduler and is in the checkpoint like any other;
  - the latest copy of each engine put in the registry, so that the others can read it;
  - the queues empty (what was not taken from them is in the log);
  - the store of the configuration at its latest version;
  - the file logs cut at the recovery line and opened to go on writing.
- What it costs and how it is checked: two copies of an engine are in memory while it is made again, and a field that the checkpoint does not hold, or that the hash does not cover, would be lost without a sign. So the acceptance test compares the hash of the restored engine with that of the replayed one and the bytes of their checkpoints, and runs both on for some ticks with the same hash.
- The replay needs the published copies by tick (the registry keeps only the latest) and the configuration by version.

Costs and risks:

- The runner is one goroutine that plays the engines in turn: replay costs about what the cluster costs live on one core, which is the machine the server targets. Running it in parallel as live does, one goroutine for each engine that waits for what it depends on, is a task of the plan.
- The log is bigger by the bodies of the messages; the benchmark of the event log records it.
- An engine added at run time needs the control stream; its shape is fixed now (R4) and the feature comes with the zonelayers that are added and removed.

The order of the work, one commit for each step, all in one pull request, each step with its tests:

1. Codecs of the actions and of the message; `OnFail` as data; `Inbound` with the actions; the record of a message that came back; dropping a message received twice.
2. The replay runner from tick 0 on the real engines with replay adapters: the whole cluster in dependency order, every recorded hash matches (`stepping.Step` is the reference run).
3. Codecs of the states, and the checkpoint of one engine: written, read, and the restored engine has the same hash, with actions waiting in the scheduler.
4. Periodic writing with offsets, each engine on its own.
5. Loading from the checkpoints of the engines with the start rule, the recovery line, and sending again what was in flight; going live by making each engine again in the real environment from a checkpoint of the replayed engine; the acceptance test of the phase.

## Making and ending engines

Agreed (phase 1) and built; see [engines](../../docs/engine_lifecycle.md). It replaced the way an engine handed its requests to its cluster inside its own tick.

- **No engine comes from nothing.** Every engine is made by a request of another engine, except the coordinator engine (the engine that holds the configuration), which the cluster makes. The coordinator engine also takes a command that asks the cluster to make or to end an engine: the way for an admin to start the engines of the game. `Cluster.CreateEngine` goes away.
- **A request is complete only when the engine that asked has taken the response.** The cluster remembers nothing of what it served: a request that is handed over again is one that was not completed, so no engine exists for it.
- **The requests and the responses go through a directive router**, a part of the cluster of its own, beside the message router: a list of requests and a list of responses for each engine, behind one mutex. Each side defines its own small interface (R1). An engine publishes its requests, takes its responses at the start of its tick and publishes an acknowledgement; the cluster takes the requests and the acknowledgements, and publishes the responses. Nothing is dropped because a queue is full. When an engine leaves the router, what waits for it is let go; the acknowledgements it published stay, since they say what it did.
- **The cluster takes them in a periodic task** (`Cluster.RunPeriodicTasks`), which also holds what `sendConfigToCoordinatorEngine` does today (now private). `Cluster.Run` calls it on a ticker, and `stepping.Step` calls it before each step. A request waits at most one period; an engine sends nothing to an engine it does not know yet, and a message to one that does not exist yet comes back as "not started".
- **A request carries the tick at which its engine made it.**

### Making an engine

A@100 is the tick 100 of the engine A that asks, B is the engine that is made.

| Step | Who | What | Record and dependency |
|---|---|---|---|
| 1 | A | an action asks (`ActionRegistry.RequestCreateEngine`); at the end of the tick the engine numbers the request, keeps it in its own data (checkpoint and hash) and publishes it, with the tick | |
| 2 | cluster | gives the ID, makes B from the state and the actions it starts with, and runs the tick 1 of B itself, as a normal tick, with B outside the message router and on no goroutine | `Init` at B@1: the state as it was made, the creator, its tick and the number of the request; depends on A@100. The start actions are for the tick 2 and later |
| 3 | cluster | publishes the response: the ID of B, or the failure | |
| 4 | A | takes the response at the start of its tick, 103 | `Directive` at A@103; depends on B@1 |
| 5 | A | at the end of the tick, after its copy is published, publishes the acknowledgement: the number of the request and the tick 103 | |
| 6 | cluster | takes the acknowledgement, puts B in the message router and runs it | `Started` at B@2: the creator and the tick 103; depends on A@103. B then runs as every engine does |

- **No cycle:** B@1 depends on A@100, A@103 on B@1, and B@2 on A@103. Every edge is a tick of an engine, so what is safe is worked out as it is for the other records (`dependencyList.Safe`), and the cache of the recovery line keeps its meaning.
- **Until the response is taken, B is not seen:** the router does not know it and no goroutine runs it. Its ID is in the response only, so no engine can read it before. There is no gate in the stable store.
- **If the engine that asked ends before it takes the response,** the cluster drops B: its log holds `Init` and nothing else.
- **If it takes the response and then ends,** B runs: the acknowledgement it published stays in the router when it leaves.
- **A request that fails** (a state type that is not registered, a start action of another state, a state that cannot be written) makes no engine and answers with the failure.

### Ending an engine

| Step | Who | What | Record and dependency |
|---|---|---|---|
| 1 | A | an action asks (`ActionRegistry.RequestRetireEngine`), as above | |
| 2 | cluster | asks B to retire, and waits until B is closed | |
| 3 | B | retires between two of its ticks | `Retired` with the engine that asked and its tick; depends on that tick. It is empty when the engine asked for itself |
| 4 | cluster | publishes the response: B ended, with the tick of its `Retired`; false when B was not there | |
| 5 | A | takes the response | `Directive` depends on `Retired` of B |

- **An engine that asks for itself to end gets no response:** it is gone. If its frame of `Retired` is lost, its request waits in its data and is handed over again.
- **The coordinator engine is never ended.**
- **What other engines read of B stays** until B is closed and removed from the store.

### Recovery

- **An engine whose log holds `Init` and nothing after it** (it never ran): at the handover it is kept and run when the log of A, as it is kept, holds the response (A took it), also when A retired after, and dropped, with its log in the archive, when the request still waits in A (it is handed over again at the first tick of A and made again with a new ID) or when the log of A holds no response (A was gone before it took one).
- **An engine whose `Started` was lost:** the ticks that depend on what it did are cut, and the cluster runs it on from `Init`.
- **A response whose `Init` was lost** is cut with the tick of A that took it, so the request waits and is handed over again.
- **Nothing is checked against a table of what was served.** `served`, `DirectiveKey`, `RestoreServed` and the periodic hand-over of the requests are not needed.

### IDs

- **The orchestrator holds the state of the IDs: the highest ID given for each type of state.** It is an object of the cluster (not an engine, and not the coordinator engine), made for the coordination of the cluster, of which the IDs are the first part. It stores the state in a file of its own in the directory of the logs, so a recovery does not read the logs from the start to find it, and it provides the IDs to the cluster. Only the cluster calls it, from the periodic task and one goroutine at a time, so it takes no lock. An engine whose ID is above what it holds is not valid and counts as not made. The numbers between are never given again, whether an engine had them or not.
- **When the file is lost or damaged, the recovery stops with an error.** A way to rebuild it is for later.

## The store: in memory first

For the storage phase, pending approval. A repository that is read often has an in-memory layer, an adapter that implements the same port as the database one, and that layer is the source of truth: it is loaded when the process starts and the database is written afterwards, by a bounded queue that keeps the order of the writes and is written out at shutdown. A write that depends on the database (a unique name, a foreign key) stays synchronous. The pattern assumes that one process writes a row; a process split brings an owner fence.

## List of ideas

| Idea | What it must achieve in project-b | Phase | Status |
|---|---|---|---|
| Entity as an interface | `ID`, `KindID`, `Position`, `Radius`, `BoundingBox`; `CharacterEntity`, `ItemEntity`; a pickup action; appear and disappear events | 0 | detail pending approval |
| Checkpoint and event log | The log is written to a file, append-only, in segments; periodic checkpoints offset between engines; recovery is the replay of the cluster from the checkpoints of the engines | 1 | agreed, the cluster-wide form pending approval |
| Early playable slice | Protobuf protocol, WebSocket gateway, TypeScript and PixiJS client, fighting, monsters, drops, pickup | 2 | agreed |
| Grid | A spatial index for fast lookup by area, and localizing the map (each entity belongs to cells by its `BoundingBox`, queries by cell) | 3 | agreed |
| Sight (AOI) | Who sees whom; entering and leaving sight; which cells changed in the tick | 3 | pending approval |
| Loading state | A new entity stays hidden until it has loaded | 3 | agreed |
| `observe` | Project events per viewer, with a no-leak test | 3 | pending approval |
| Performance gate | One full layer of 150 entities and 4 layers in parallel, p99 of a tick under 45 ms per layer; if not met, stop and fix the effect design | 5 | agreed |
| Gameplay designs | Death, respawn, offline cultivation; seasons; localization (text key and parameters); hit resolution under network lag | 4 to 10 | agreed into the plan |
| `effects` | Effect framework, registry by ID, standing actions by scope, 10 sample abilities | 5 | pending approval |
| Metrics | The engine keeps plain numbers (players, capacity, tick time histogram) and does not know Prometheus | 1 | pending approval |
| Load tool | Bots that log in, walk and chat; load measurement scenarios | 8 | pending approval |
| Tiled map reading | Load the map, spawn points, destinations and exits from a file | 3 | pending approval |
| Zonelayer on demand, layer change | A zone creates a layer only when someone arrives; move people between layers | 1 | pending approval |
| Engines made and ended by requests | An engine asks for another to be made or ended through the directive router; the cluster makes it in two steps (`Init`, then `Started` when the engine that asked took the response); no engine is made from nothing | 1 | agreed |
| Write-behind queue | Writing to the store does not block a tick | 6 | pending approval |
| Character names | Unicode normalization, allowed alphabet, duplicate check, banned words | 6 | pending approval |
