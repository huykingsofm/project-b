# How the simulation works

The simulation core touches nothing outside: no gateway, no database, no network. Outside code gives it commands and takes the events. Code: [`platform`](../platform) (what runs in ticks, whatever the game) and [`server/game`](../server/game) (this game). Actions: [action/index.md](action/index.md). Entities: [entity/entity.md](entity/entity.md).

## Platform and game

| Layer | Holds | Code |
|---|---|---|
| platform | the interfaces `State`, `Action[S]`, `Command[S]`, `Message`; the plan, the scheduler, the registry an action is given, the codecs; the engine `Engine[S]` that runs any state; the router, the store of published states, the event log and the replay runner | `platform/sim`, `platform/engine`, `platform/eventlog` |
| game | the three states below, their events, the actions and commands that change them, the tile map, and the commands and actions that make the zones and zonelayers (`WorldZoneAdd`, `WorldZoneLayerAdd`) | `server/game` |
| shared | libraries that import nothing of the project | `shared` |

- The platform imports nothing of the game (R1, R2). The game imports the platform, and `server/cmd` wires them.
- The platform knows no world, zone or zonelayer: there is one engine, `Engine[S]`, for any type of state `S`, and a message, a command or an action of one type of state cannot reach an engine of another (the compiler checks it where the types are known, the codecs where they are not).
- Every state has an ID of its own, a `sim.StateID`, made of the type of the state (8 bits) and a number among the states of its type (56 bits, counted up and never given again), so it is unique among all states and says what type its state is; it is the address of a message and the key that another engine reads the published state by. `sim.StateType` says what kind of state it is.

## States of this game

| State | Holds | State package |
|---|---|---|
| world | private messages between players, inventories, every zone and zonelayer of the world | `server/game/sim/state/world` |
| zone | the world it is in, the map, the exit entities (each says the zone it leads to), the zonelayers and capacity, zone chat | `server/game/sim/state/zone` |
| zonelayer | the zone and the world it is in, one copy of a zone's map with its entities, cooldowns, chat | `server/game/sim/state/zonelayer` |

- Each state has events, a plan, actions and an engine of its own.
- States never share data (R28): what another state holds is read from its published state by its ID (`Registry.Get[T]`), or asked for in a message. Any engine finds the others through the world, which lists them.

## State and events

- A state is data. Every field of a state, and of every value it holds (a character, an entity, a message, an inventory slot), is private: it is read with a method and changed only by a method that the state offers, and the methods that change it write the events. A value is made by a constructor (`NewCharacter`, `NewChatMessage`, `NewPrivateChatMessage`) and its setters are for what is not yet in a state.
- A state implements `sim.State`: `Type()`, `Tick()`, `NextTick()`, `Clone()` (a copy for the other engines to read, behind the interface), `Hash()`, `Encode` and `TakeEvents()`. It holds no ID of its own: the ID is the engine's (`Context.Self` for an action, the header of a checkpoint).
- The events are the list of what a client should hear (`sim.Events[E]` holds a `[]E`). They are not cloned or published.
- Every event has `Audience()`:

| Audience | Who receives |
|---|---|
| Everyone | everyone who watches the scope |
| Characters | a list of characters |
| Players | a list of players |
| InSightOf | whoever can see an area; the area of an entity's event is its position or path grown by the entity's radius |

- A private event is never addressed to everyone.
- Events do not name connections; whatever sends events to clients, or to an NPC that must decide, resolves the audience.
- Every datum in a state is a value or has `Clone` (R29).

## One tick of an engine

`platform/engine/engine.go`, `Engine[S].Step`:

0. Take the state to the next tick (`NextTick`); when the state does not then say the tick of the engine, return an error that wraps `ErrTickMismatch`, having done nothing else.
1. Read the configuration once, from the copy of the configuration engine that the stable view keeps for the tick ([configengine](configengine.md)); decide on what came back undelivered.
2. Build the plan: what was scheduled for this tick (what the engine set for itself in the last tick included), the messages of other engines, the commands.
3. Run the plan, one action after another, on the state, in place.
4. Tell the actions that were cancelled; put what the actions set for the engine in the scheduler.
5. Hand the events to the emitter.
6. Publish a `Clone` of the state (other engines read the latest copy).
7. Send the messages the actions put in the outbox; a message that cannot be delivered is decided by its own `OnFail`; send again the unsettled messages that went and were not acked ([unsettled messages](unsettled_messages.md));
8. Write the hash when it is due, and close the tick for the event log; say to the senders what is safe.

| When a tick fails | What happens |
|---|---|
| `stepping.Step` | the router stops (every message bounces as `DeliveryFailureStopping`), the engines after the one that failed do not run, and the error is returned, wrapped |
| `Cluster.Run` | the engine that failed retires and returns its error; every other engine stops as if the context had ended; the error is logged once, by `Run` |
| an engine is made | it starts at the tick of its state; `DecodeEngineCheckpoint` and `EngineCheckpoint.Restore` refuse a checkpoint whose state is not at the tick in its header |

## Actions

- Never fail, never leave their state; what must happen outside goes in the registry ([registry](action/index.md#registry)).
- An action that must wait sets itself at the tick it knows it can run.
- The plan is indexed by the ID of its actions and by their author, so an action can find others without walking the plan; [`ZoneLayerCharacterShieldGrantAction`](action/zonelayer.md#zonelayercharactershieldaction) is the pattern.
- A cancellable action implements `sim.Cancellable[S]` (a `sim.Action[S]` with one more method, `ApplyCancel`).
- Every action is a value type, with its methods on the value ([R29](../plan/rule/r29-clone-or-value.md)): an action that changes as it runs sets a changed copy of itself for a later tick. Its `Clone` returns itself, and one that holds a character clones the character: a checkpoint and an engine that takes a message copy actions with it.
- Cooldowns: [action/index.md](action/index.md#cooldowns).

## Reading other states

- No state keeps a copy of another's data or a count of its characters; see [action/index.md](action/index.md#reading-other-states).
- The map belongs to the zone (`tilemap.TileMap` in the zone state).
- An action that needs the map and finds none does nothing that tick; what it does next is its own rule (move and enter try again next tick, an exit does nothing).

## Delivery of messages

- A message decides what a failed delivery means with its `OnFail`: [action/index](action/index.md#failed-delivery).
- The kind runs itself (`OnFail.Handle`) on the bouncer of the engine (`platform/engine/bounce.go`), through `sim.Bouncer`.
- A message and its `OnFail` are written by one codec (`platform/sim/codecs.go`): [eventlog](eventlog.md#messages-and-actions).
- The engine that takes a message runs a clone of it (`Message.Clone`: each action and the `OnFail` with their `Clone`): the sender keeps the message until it is acked, and an action may change what it holds as it runs.

## Chat

| Kind | Where it stays | Action |
|---|---|---|
| zonelayer | the zonelayer | [`ZoneLayerChatSendAction`](action/zonelayer.md#zonelayerchatsendaction) |
| zone | recorded by the zone, given to each zonelayer | [`ZoneChatSendAction`](action/zone.md#zonechatsendaction) |
| private | the world, between players | [`WorldChatPrivateSendAction`](action/world.md#worldchatprivatesendaction) |

## Crossing zones

- An exit is optimistic: the character moves at once and does not wait for the other zonelayer; see [`ZoneLayerCharacterExitToAction`](action/zonelayer.md#zonelayercharacterexittoaction).
- Two actions, exit-to and enter; no state is kept for the road between.

## Engine

`platform/engine`; the cluster of this game is made by the code that runs it (`cmd/world`, `server/game/testkit`): it adds the world, and the world asks for the rest ([engine lifecycle](engine_lifecycle.md)).

| Part | What it is |
|---|---|
| `Engine[S]` | one per state, each on its own goroutine; the same code for every type of state |
| `Cluster`, `Environment` | the engines, in the order they were added, and what they share (the router, the published states, the log, the codecs, the registry of engines, the seed) |
| `engine.EngineRegistry` | how an engine is made from a state, or from a checkpoint, of each type; the game registers its types in `game.NewCodecsAndEngines` |
| `stepping.Step` | runs one tick of every engine, one after the other, for a script or a test that needs the same run every time |
| `Router` | carries messages between engines through `OutgoingChannel` |
| `peer` | what an engine has to talk to the other engines: the `IncomingChannel`, the router, the `StableView`, the bouncer, the numbers of the messages, the unsettled messages and the acks; the engine itself holds its random numbers, its configuration and its event log |
| `StableStateStore` | the latest published state of every engine, whatever its type |
| `StableView` | what one engine reads of the store; fixed within a tick, first read of each state written to the event log |
| `Emitter` | one; the events of every engine, which the game interprets |
| `eventlog.Log` | records the input from outside and the state hash (sparse, offset per engine, and at each checkpoint); `EndTick` says an engine is done with a tick; `eventlog.MemLog` keeps the records in memory, `eventlog.FileLog` writes them to files ([eventlog](eventlog.md)) |
| `CheckpointWriter` | writes the checkpoints that the engines take at the end of a tick, out of their goroutines; `checkpoint.FileWriter` writes them to files ([checkpoint](checkpoint.md)) |
| `PlanPool[S]` | one per type of state |
| `sim.Codecs` | the registers that read back states, actions, commands and kinds of `OnFail`; the game fills them (`game.NewCodecsAndEngines`) and puts them in `Environment` |

## Plan

- The plan is a `table.PriorityQueue` of its entries: priority (smaller first), then arrival; a running pointer; a column by ID. An engine makes it in one go with `Build` from the actions a tick starts with, and an action that runs adds what it makes with `Append`, which is ordered among the entries still to run and never before the one that runs. `FindByID` walks the actions of an ID as a type, lazily and with no allocation; `Find` returns a slice. Both give the entries that ran or were cancelled as well as the ones to run, and whoever changes an action checks `Status()` of the entry (`EntryPending`, `EntryRan`, `EntryCancelled`, `EntryCut`). `Cancel` of an entry that is pending sets its status and lists it in `Cancelled`, which the engine reads to call `ApplyCancel`; `Next` passes over what is not pending.
- A depth limit on chains of actions.
- A scheduler (an action set for a tick). What repeats is an action that sets itself again; what an action sets for the next tick waits in it as well. Outside the engine, `Engine.Scheduler()` is a `sim.ReadOnlyScheduler`: it finds the action of an ID that waits for the earliest tick (`FindByID`) and says how many wait (`Pending`); only the engine changes it.
- Messages with `OnFail`.

## Shared libraries and configuration

| Package | What it is |
|---|---|
| `shared/stablemap` | map with a stable order of visit: the same for the same calls, though not the order of insertion once a key is deleted (R20) |
| `shared/table` | `Table[K, V]`: a stablemap with columns, values found by one key, or by two (the first alone or both), found by the type of the key (`Index`, `Index2`, `Lookup`, `LookupAs`, `Lookup2`, `Lookup2First`); `PriorityQueue[V, P]` (`V` says its own priority, `Priority() P`, as a `Prioritier[P]`; the zero queue is ready): elements taken by priority and then by arrival, which remembers what it held in the round, with the same columns on all of it (`Add`, `BuildFrom`, `Next`, `All`, `Pending`) |
| `shared/ring` | the last rows of a log that only grows at the end (the chat) |
| `shared/registry` | the latest published copy under a key |
| `shared/codec` | a register of kinds told apart by a number, each with the function that reads it; and `Version`, the version of the data that a `Decode` reads, for the event log; refuses a number or a Go type that two kinds share |
| `shared/bimap` | two-way map (the zonelayer state links characters and bodies with it) |
| `shared/channels` | `Drain` |
| `shared/logging` | key=value log lines; a call only fills a channel ([logging](logging.md)) |
| `shared/bitset` | K bits, the walls of a `TileMap` ([bitset](bitset.md)) |
| `shared/segmentlog` | append-only log in segment files, under the event log ([segmentlog](segmentlog.md)) |
| `shared/codec/binary` | integers (checked against the type they are read as) and strings as bytes: `Writer` and `Reader` |
| `shared/errs` | `Join`: joins what many registrations returned into one error |
| `shared/clone` | the contract for copying: a value, or a type with `Clone`; the containers use it ([R29](../plan/rule/r29-clone-or-value.md)) |
| `shared/testing/aliasing` | for tests: finds memory that two values share, and checks that a type is a value or has `Clone` |
| `platform/config` | `Var[T]` with defaults and limits; an immutable snapshot, read as a `ReadOnlySnapshot`; a whole set is rejected when one value is wrong |

## Temporary values

- The damage of the normal attack (1) and the regen (1) are written in the code; formulas replace them later.

## Replay

- Needs the inputs from outside and what only the time decides (`platform/eventlog/`); what engines send themselves is made again from the state.
- The hash an engine records is the hash of its state, of what it waits for and of what only it knows ([checkpoint](checkpoint.md#hash)).
- An engine records, in its own log: the messages it took (whole), the messages it sent that came back and the reason, the commands of clients, the hash of its state every few ticks, and which copy of another engine it read first.
- In files: [eventlog](eventlog.md).
- Run again from the records, on the real engines with other adapters: [replay](replay.md). Cut to the recovery line and run again from the checkpoints: [replay](replay.md).
