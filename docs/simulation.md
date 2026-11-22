# How the simulation works

The simulation core touches nothing outside: no gateway, no database, no network. Outside code gives it commands and takes the events. Code: [`platform`](../platform) (what runs in ticks, whatever the game) and [`server/game`](../server/game) (this game). Actions: [action/index.md](action/index.md). Entities: [entity/entity.md](entity/entity.md).

## Platform and game

| Layer | Holds | Code |
|---|---|---|
| platform | the interfaces `State`, `Action[S]`, `Command[S]`, `Message`; the plan, the scheduler, the registry an action is given, the codecs; the engine `Engine[S]` that runs any state; the router, the store of published states, the replay log and the replay runner | `platform/sim`, `platform/engine`, `platform/replay` |
| game | the three states below, their events, the actions and commands that change them, the tile map, the builder of the cluster | `server/game` |
| shared | libraries that import nothing of the project | `shared` |

- The platform imports nothing of the game (R1, R2). The game imports the platform, and `server/cmd` wires them.
- The platform knows no world, zone or zonelayer: there is one engine, `Engine[S]`, for any type of state `S`, and a message, a command or an action of one type of state cannot reach an engine of another (the compiler checks it where the types are known, the codecs where they are not).
- Every state has an ID of its own, a `sim.StateID`, made of the type of the state (8 bits) and a number among the states of its type (24 bits), so it is unique among all states and says what type its state is; it is the address of a message and the key that another engine reads the published state by. `sim.StateType` says what kind of state it is.

## States of this game

| State | Holds | State package |
|---|---|---|
| world | private messages between players, inventories, every zone and zonelayer of the world | `server/game/sim/state/world` |
| zone | the world it is in, the zones its exits lead to, the map, the exit entities, the zonelayers and capacity, zone chat | `server/game/sim/state/zone` |
| zonelayer | the zone and the world it is in, one copy of a zone's map with its entities, cooldowns, chat | `server/game/sim/state/zonelayer` |

- Each state has events, a plan, actions and an engine of its own.
- States never share data (R28): what another state holds is read from its published state by its ID (`Registry.Get[T]`), or asked for in a message. Any engine finds the others through the world, which lists them.

## State and events

- A state is data. Every field of a state, and of every value it holds (a character, an entity, a message, an inventory slot), is private: it is read with a method and changed only by a method that the state offers, and the methods that change it write the events. A value is made by a constructor (`NewCharacter`, `NewChatMessage`, `NewPrivateChatMessage`) and its setters are for what is not yet in a state.
- A state implements `sim.State`: `ID()`, `Type()`, `String()`, `Tick()`, `SetTick`, `Clone()` (a copy for the other engines to read, behind the interface), `Hash()`, `Encode` and `TakeEvents()`.
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

1. Take the configuration once; decide on what came back undelivered.
2. Build the plan: what the engine set for itself, what was scheduled for this tick, the messages of other engines, the commands.
3. Run the plan, one action after another, on the state, in place.
4. Settle the actions that were cancelled; take what the actions set for the engine.
5. Hand the events to the emitter.
6. Publish a `Clone` of the state (other engines read the latest copy).
7. Send the messages the actions put in the outbox; a message that cannot be delivered is decided by its own `OnFail`.

## Actions

- Never fail, never leave their state; what must happen outside goes in the registry ([registry](action/index.md#registry)).
- An action that must wait sets itself at the tick it knows it can run.
- The plan is indexed by the ID of its actions and by their author, so an action can find others without walking the plan; [`ZoneLayerCharacterShieldGrantAction`](action/zonelayer.md#zonelayercharactershieldaction) is the pattern.
- A cancellable action implements `sim.Cancellable[S]` (a `sim.Action[S]` with one more method, `ApplyCancel`).
- Cooldowns: [action/index.md](action/index.md#cooldowns).

## Reading other states

- No state keeps a copy of another's data or a count of its characters; see [action/index.md](action/index.md#reading-other-states).
- The map belongs to the zone (`tilemap.TileMap` in the zone state).
- An action that needs the map and finds none does nothing that tick; what it does next is its own rule (move and enter try again next tick, an exit does nothing).

## Delivery of messages

- A message decides what a failed delivery means with its `OnFail`: [action/index](action/index.md#failed-delivery).
- The kind runs itself (`OnFail.Handle`) on the bouncer of the engine (`platform/engine/bounce.go`), through `sim.Bouncer`.
- A message and its `OnFail` are written by one codec (`platform/sim/codecs.go`): [replay_log](replay_log.md#messages-and-actions).

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

`platform/engine`; the cluster of this game is built in `server/game`.

| Part | What it is |
|---|---|
| `Engine[S]` | one per state, each on its own goroutine; the same code for every type of state |
| `Cluster`, `Env` | the engines, in the order they were added (the order of `Step`), and what they share |
| `game.ClusterBuilder` | `NewBuilder`, `With...`, `Build`: builds the cluster of this game; gives every state its ID |
| `Router` | carries messages between engines through `OutgoingChannel` |
| `cell` | holds the `IncomingChannel`, the config, the replay log and the bouncer |
| `StableStateStore` | the latest published state of every engine, whatever its type |
| `StableView` | what one engine reads of the store; fixed within a tick, first read of each state written to the replay log |
| `Emitter` | one; the events of every engine, which the game interprets |
| `replay.Log` | records the input from outside and the state hash (sparse, offset per engine); `EndTick` says an engine is done with a tick; `replay.MemLog` keeps the records in memory, `replay.FileLog` writes them to files ([replay_log](replay_log.md)) |
| `PlanPool[S]` | one per type of state |
| `sim.Codecs` | the registers that read back states, actions, commands and kinds of `OnFail`; the game fills them (`game.NewCodecs`) and puts them in `Env` |

## Plan

- Priority, a running pointer, an index by ID and by author. `FindByID` walks the actions of an ID as a type, lazily and with no allocation; `Find` returns a slice.
- A depth limit on chains of actions.
- A scheduler (an action set for a tick). What repeats is an action that sets itself again; the scheduler finds and cancels what waits by a key (the ID of the action and its actor), and an action cancels through its registry (`CancelWaiting`), which takes effect when the tick is over.
- Messages with `OnFail`.

## Shared libraries and configuration

| Package | What it is |
|---|---|
| `shared/orderedmap` | map with fixed iteration order (R20) |
| `shared/table` | rows that define `Index()`, dirty, `FindBy(column, value)` |
| `shared/registry` | the latest published copy under a key |
| `shared/codec` | a register of kinds told apart by a number, each with the function that reads it; refuses a number or a Go type that two kinds share |
| `shared/bimap` | two-way map (the zonelayer state links characters and bodies with it) |
| `shared/channels` | `Drain` |
| `shared/logging` | key=value log lines; a call only fills a channel ([logging](logging.md)) |
| `shared/bitset` | K bits, the walls of a `TileMap` ([bitset](bitset.md)) |
| `shared/segmentlog` | append-only log in segment files, under the replay log ([segmentlog](segmentlog.md)) |
| `shared/binarycodec` | integers and strings as bytes, and `Version`, the version of the data that a `Decode` reads, for the replay log |
| `shared/clone` | the contract for copying: a value, or a type with `Clone`; the containers use it ([R29](../plan/rule/r29-clone-or-value.md)) |
| `shared/testing/aliasing` | for tests: finds memory that two values share, and checks that a type is a value or has `Clone` |
| `platform/config` | `Var[T]` with defaults and limits; a snapshot taken once per tick; a whole set is rejected when one value is wrong |

## Temporary values

- The damage of the normal attack (1) and the regen (1) are written in the code; formulas replace them later.

## Replay

- Needs the inputs from outside and what only the time decides (`platform/replay/`); what engines send themselves is made again from the state.
- The hash an engine records is the hash of its state, of what it waits for and of what only it knows ([checkpoint](checkpoint.md#hash)).
- An engine records, in its own log: the messages it took (whole), the messages it sent that came back and the reason, the commands of clients, the version of the config it first read, the hash of its state every few ticks, and which copy of another engine it read first.
- In files: [replay_log](replay_log.md).
- Run again from the records, on the real engines with other adapters: [replay_runner](replay_runner.md).
