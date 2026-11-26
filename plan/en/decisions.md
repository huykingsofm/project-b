# Decisions of the owner

## Decided

| Topic | Decision |
|---|---|
| Phase order | The client comes early: phase 2 is a playable slice (fight, pick up, drop, monsters); the later phases run in parallel with the frontend |
| Parallel work | The owner decides by opening separate sessions and branches; the plan only states dependencies |
| Performance | 2,000 players at once on one machine (one machine is one world), p99 of a tick under 20 ms at the network layer and under 45 ms per layer; these are minimums |
| Entities | An interface: `ID`, `KindID`, `Position`, `Radius`, `BoundingBox`; collision by radius; `CharacterEntity` and `ItemEntity` first |
| Data in the simulation | Every piece of data is a value or has a `Clone` (R29) |
| Owner of data | A piece of data belongs to exactly one engine; remove first, then send; save after the tick ends (R28) |
| Replay | No replay of a single engine; checkpoints of the whole state of some engines at a period, offset between engines to avoid CPU peaks |
| `seq` of messages | Left as it is |
| Business examples | Allowed in the code as examples, to expose a design that does not fit early; a base game can change them later |
| Protocol | Fast and light: protobuf with `buf`, binary WebSocket frames |
| Event log | An append-only file like Kafka's, with checkpoints; format as in [proposed-designs.md](proposed-designs.md); written to a file, pushed to S3 later (low priority, phase 6) |
| Recovery | Option A: every engine loads its own latest checkpoint and replays its own input; reads of another engine's state (`StableRead`) are not recorded by value, so after a failure they run against the recovered states. A verifying replay from tick 0 still uses the hash |
| Checkpoint period | A config parameter like the hash, but much rarer than the hash |
| R4 | Fixed: business examples are allowed when marked as examples and replaceable without changing the mechanism |
| Production environment | Not now; docker compose only |
| Important data | Postgres; generated queries (sqlc), migrations (goose) |
| Password hashing | argon2id; session token; authenticate once when the WebSocket opens, later messages are not authenticated again |
| Web client | TypeScript, PixiJS and Vite; stable, fast and easy to write |
| Observability | Not needed now; in the plan at low priority (phase 8) |
| Deployment | docker compose |
| Records | No ADRs in project-b; no `depguard` |
| Containers in a state | `stablemap` (stable order of visit, which is not the order of insertion after a delete), `table.Table[K, V]` (a stablemap with columns of one key or two, found by the type of their key, never changed in place), `table.PriorityQueue[V, P]` (the element says its priority itself, as a `Prioritier[P]`; elements taken by priority and then by arrival, which keeps what it held in the round, the taken included; the plan of a tick is one, and its entries say if they ran or were cancelled), `ring.Ring` for a log that only grows (the chat); no `Table` of rows |
| Entities | `Entity`, `DamageableEntity` and the list of `KindID` are common, in `state`; the entities of each state are in a package `entity` below it, built with constructors, with a codec of their own |
| Access to a state | Every field of a state, and of every value it holds, is private; a state is read and changed only through its methods, and a state implements `sim.State` with `ID()`, `Tick()` and `Clone() sim.State` |
| Platform and game | Two layers: `platform` (what any game that runs in ticks is made of: `State`, `Action[S]`, `Command[S]`, `Message`, plan, scheduler, registry, codecs, the engine `Engine[S]`, the event log and runner) and `server/game` (this game: the world, zone and zonelayer states, their actions and commands, the cluster builder); `shared` at the root. The platform has no world, zone, zonelayer, graph or scope: every state has a `StateID` made of its `StateType` (8 high bits) and a number local to the type (24 bits), so it is unique among all states and says its type (the address of a message and the key of a read); each state keeps the IDs of the states it needs, and the world lists them all; an `EntityID` is made of the ID of the state that holds it and a number it counted; an action reads another state with `Registry.Get[T](id)`; the codecs are registers that the game fills and puts in `Env`, and a log file is named after the name of the type of its state and the ID, with the number of the type in its meta |

## Still open

Nothing blocks the work that is planned now. The choices below are for later, when monsters and other NPCs are built (phase 9); each needs the owner's decision first.

### Telling clients what changed (priority high, near future)

What is already decided: every event declares its audience; positions, radii and distances are in units and only the map knows tiles (a tile is 1024 units, a power of two, so scaling and dividing by a tile can be shifts); speed belongs to the character and is in units a tick; an entity ID stays one counter for each scope, with no slot and no generation (a command is addressed to its zonelayer by the session, and events and fields say which scope an ID is of).

The problem: with N entities that move and V viewers in the same crowd, telling every viewer about every move costs N times V per tick. A layer holds about 150 characters, so one crowded layer is up to 22,500 deliveries a tick, and it grows with everything that moves (monsters, projectiles). Units do not add events (an entity already moves once a tick); they make every position different, so a move is less often the same as the one before.

| Choice | Options | What it changes |
|---|---|---|
| Who sees what | A grid of cells: each event is filed under its cells once, and a viewer takes the cells around it | The cost drops from N times V to about N times the number of viewers per cell; needs the grid of this phase |
| Moves | One `EntityMoved` per entity per tick (first `From`, last `To`) instead of one per step | Fewer events when several actions move the same entity |
| Send rate | Near entities every tick, far ones every few ticks | Bandwidth and CPU in a crowd, at the price of rougher far movement |
| Visible limit | A viewer is told about at most M entities, the nearest or the most relevant | A hard bound on the cost per viewer |
| Crowds | Events per cell in a crowd are sent as one snapshot of positions that changed, not as many events | Cheaper for a mass fight, more work to design |
| Who is known | Each viewer keeps the set of entities it already knows, so entering and leaving sight is a difference of sets | Correct appear and disappear |
| Packets | One packet per viewer per tick | Network cost and the protocol |
| Different views | Two events with two audiences (decided as the default; see the phase text) | The no-leak test, and every response stays shared |

### How events reach clients

Every event declares its audience, and an audience resolves to channels. A channel is a key and the list of events of a tick that belong to it.

| Audience | Channel | Built | Sent to |
|---|---|---|---|
| Everyone (world, zone or zonelayer) | the public channel of that scope | once for the whole scope | every connection in the scope, the same bytes |
| In sight of an area | a grid block, one for each block the area covers | once for each block | the viewers that see the block |
| Characters or players | a private channel of each recipient | for each recipient | that recipient only |
| A group (a sect, later a sect inside a zone) | the channel of the group | once for the group | the members of the group, in the zone when the group is limited to one |

How it runs:
- The write phase is in the engine at emit time: for each event, in order, the audience resolves to channel keys and the event is appended to each channel (by reference, never copied). It is a few appends, so it fits in the tick.
- The send phase is on the gateway side, off the goroutine of the engine: each channel's list is encoded once, and each viewer is sent the shared responses of the channels it listens to, plus its own private one. The response of a public channel is the same for everyone, so it is built once and broadcast.
- A viewer listens to the public channels of its world, zone and zonelayer, the blocks around it, its private channel and its groups. Moving changes the blocks it listens to; when a block is gained, the viewer also gets the entities that are in it, and the ones in a block it left are removed (the known-sets).
- A viewer may receive several responses in a tick (shared ones and its own). Each carries the tick and a sequence number of its events, so the client can put them in order.
- The audience kinds are a table, so a new kind of group is a new entry and the code that routes and sends does not change (R8, R0).

### AI for NPCs and monsters

| Choice | Options | What it changes |
|---|---|---|
| Decision model | State machine (FSM); behavior tree; utility scoring; GOAP; a mix | How much a designer can add without code, and how complex a monster can be |
| Where the AI runs | Inside the zonelayer engine as a scheduled action that produces actions; or an engine of its own that sends commands by message | Tick cost per layer (R28, the 45 ms budget), and whether the AI reads a state that is a few ticks old |
| What the AI may do | Only the actions a player's command makes (R7); or extra NPC-only actions | Fairness, and whether a client can show an NPC exactly like a player |
| Perception | Which events an NPC receives (all of its zonelayer, or only what it sees), sight and hearing range, memory of what it saw | Depends on the audience of events and on sight (phase 3) |
| Aggro and threat | One target; or a threat table; how threat is gained, lost and shared; leash distance and reset rules | How kiting, tanking and pulling behave |
| Thinking rate | Every tick; every N ticks; N depending on distance from players | CPU for 2,000 players and many NPCs |
| Sleeping | Whether and when an NPC with no player nearby stops thinking or stops existing | CPU, and what a returning player finds |
| Movement | Straight to the target; grid A*; navmesh; flow field; steering against crowding | Cost, and how sub-tile movement and walls are used |
| Spawning | Spawn points, respawn time, a limit of NPCs per layer; which scope owns the spawn state (zone or zonelayer); what happens when a layer is empty | R28 ownership, and the number of entities per layer |
| Group behavior | Packs, summons, bosses with phases, scripted events | Whether the AI of one NPC may read or command another |
| Data or code | NPC behavior described in data files and reloaded while running; or written in code | Who can change a monster, and how it is tested |
| Randomness and replay | All AI randomness from the seeded RNG of the engine, so a replay reproduces it without logging decisions | Replay and the hash (R20) |
| Lag and fairness | How far an NPC attack waits for the client's own timing (R16) | How fights feel at high latency |
| Debugging | A way to see why an NPC did what it did (state, scores, target) | Tools (phase 8) |

### The map of a zone and the checkpoint (for later, near the phase of the replay of the cluster)

Today the checkpoint of a zone does not hold its map, and `zone.DecodeState` takes a function `tilesOf(zoneID)` that gives the map back. The owner does not like it: the decoder of this one state has a signature of its own, the closure in `NewCodecs`, a `ClusterBuilder` that holds the zones only to make that function, and a checkpoint that is read with a map that may have changed since it was written, with no one telling.

| Way | What it is | Gives | Costs |
|---|---|---|---|
| Write the map in the checkpoint | `Encode` writes the map, `DecodeState(r, version)` is like every other | no `tilesOf`, no zones held by the builder, and the map goes with the checkpoint | the size of the map for each checkpoint of each zone: about 60 a hour, so 7.5 MB an hour for a map of 1,000 by 1,000 tiles before any compression; fine for a small map |
| Write the map in compressed runs | the same, with the blocked tiles as runs | most of the cost of the first way is gone | a codec of runs for the bitset |
| Separate static data from the state | the map is not a field of the state: the engine holds it, as it holds the configuration, and an action reads it through the context | the right model; every state is read by `DecodeState(r, version)` | the platform has to carry data of the game; `NewState` and every action that reads `TileMap()` change |
| A reference in the log (the way the owner proposed) | a format for reading by reference, `KindRef`: a file name and the number of a record in it; the reader opens that file at that record and takes out a record of its own kind and body, as usual | generic: any big body (a map, a message sent many times, a copy that many engines read) is written once; the log and the checkpoint stay small | a record can be reached by its number (an index of `segmentlog`, which is read in order today); the target is written and committed before the reference; a reference to a reference is refused or its depth is limited; what is referenced is never deleted; `Body.Same` compares what a reference gives |

- **Where a reference is read:** the checkpoint is not a record of the log, so the format has to be under both: `binary.Writer.Ref(file, n)` and a `binary.Reader` that is given a function that resolves a reference. Then `DecodeState(r, version)` keeps one signature, and what loads the map is the reader, not the decoder of the zone.
- **Not decided:** who writes the map once and gives its address (file and record) to the state at `Encode`; this needs a place where the cluster registers its assets when it starts.
- **Decision:** not made. The size of the real maps decides between the first ways and the last two.

### The limit of the messages that went and were not acked (before it is built)

The design is in [proposed designs](proposed-designs.md#the-limit-of-the-messages-that-went-and-were-not-acked-pending-approval); each choice below needs the owner's decision first.

| Choice | Options | What it changes |
|---|---|---|
| The default and the range of `unsettled_messages.most_per_destination` | a number near the queue of an engine (256 by default) times a few, or one chosen after the checkpoint is measured | how much a slow receiver can take before its senders refuse, and the largest checkpoint |
| What counts against the limit | the sent messages only (proposed), or the sent and the unsent | the unsent ones are the failures that wait to go again: counting them stops that list growing too, and makes a destination that refuses look fuller than what is in its channel |
| How an engine of a replay knows it must not decide | `Env.RefusesWhenUnsettledIsFull`, a value of `Env` like `Acks` and `Checkpoints` (proposed), or a method of `MessageRouter` that the replay router answers from the log | where the rule lives: in the engine, which the replay has to turn off, or in the router, which every router has to implement |
