# How the simulation works

The simulation core touches nothing outside: no gateway, no database, no network. Outside code gives it commands and takes the events. Code: [`server/internal/services/game`](../server/internal/services/game). Actions: [action/index.md](action/index.md). Entities: [entity/entity.md](entity/entity.md).

## Scopes

| Scope | Holds | State package |
|---|---|---|
| world | private messages between players, inventories | `sim/state/world` |
| zone | the map, the exit entities, the zonelayers and capacity, zone chat | `sim/state/zone` |
| zonelayer | one copy of a zone's map with its entities, cooldowns, chat | `sim/state/zonelayer` |

- Each scope has a state, events, a plan, actions and an engine of its own.
- Scopes never share data (R28): what another scope holds is read from its published state, or asked for in a message.

## State and events

- A state is data. Its methods are the only way to change it, and they write the events.
- The events are the list of what a client should hear (`state.Events[E]` holds a `[]E`). They are not cloned or published.
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

`engine/zonelayer.go`, `Step`:

1. Take the configuration once; decide on what came back undelivered.
2. Build the plan: what the engine set for itself, what was scheduled for this tick (and the crons), the messages of other engines, the commands.
3. Run the plan, one action after another, on the state, in place.
4. Settle the actions that were cancelled; take what the actions set for the engine.
5. Hand the events to the emitter.
6. Publish a `Clone` of the state (other engines read the latest copy).
7. Send the messages the actions put in the outbox; a message that cannot be delivered is decided by its own `OnFail`.

## Actions

- Never fail, never leave their scope; what must happen outside goes in the registry ([registry](action/index.md#registry)).
- An action that must wait sets itself at the tick it knows it can run.
- The plan is indexed by the ID of its actions and by their author, so an action can find others without walking the plan; [`ZoneLayerCharacterShieldGrantAction`](action/zonelayer.md#zonelayercharactershieldaction) is the pattern.
- A cancellable action implements `ZoneLayerCancellableAction` (a `ZoneLayerAction` with one more method).
- Cooldowns: [action/index.md](action/index.md#cooldowns).

## Reading other scopes

- No scope keeps a copy of another's data or a count of its characters; see [action/index.md](action/index.md#reading-other-scopes).
- The map belongs to the zone (`tilemap.TileMap` in the zone state).
- An action that needs the map and finds none does nothing that tick; what it does next is its own rule (move and enter try again next tick, an exit does nothing).

## Delivery of messages

- A message decides what a failed delivery means with its `OnFail`: [action/index](action/index.md#failed-delivery).
- The kind runs itself (`OnFail.Handle`) on the bouncer of the engine (`engine/bounce.go`), through `message.Bouncer`.
- A message and its `OnFail` are written by one codec (`action/codec.go`): [replay_log](replay_log.md#messages-and-actions).

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

`server/internal/services/game/engine`.

| Part | What it is |
|---|---|
| Engine | one per scope, each on its own goroutine |
| `ClusterBuilder` | `NewBuilder`, `With...`, `Build`: builds the cluster |
| `Router` | carries messages between engines through `OutgoingChannel` |
| `cell` | holds the `IncomingChannel`, the config, the replay log and the bouncer |
| `StableStateRegistry` | the latest published state of every engine |
| `StableStateView` | what one engine reads of the registry; fixed within a tick, first read of each scope written to the replay log |
| `Emitter` | three (world, zone, zonelayer); send events out |
| `replay.Log` | records the input from outside and the state hash (sparse, offset per engine); `EndTick` says an engine is done with a tick; `replay.MemLog` keeps the records in memory, `replay.FileLog` writes them to files ([replay_log](replay_log.md)) |
| `PlanPool` | one per scope |

## Plan

- Priority, a running pointer, an index by ID and by author. `FindByID` walks the actions of an ID as a type, lazily and with no allocation; `Find` returns a slice.
- A depth limit on chains of actions.
- A scheduler (an action set for a tick) and a cron (an action repeated every N ticks).
- Messages with `OnFail`.

## Shared libraries and configuration

| Package | What it is |
|---|---|
| `shared/orderedmap` | map with fixed iteration order (R20) |
| `shared/table` | rows that define `Index()`, dirty, `FindBy(column, value)` |
| `shared/registry` | the latest published copy under a key |
| `shared/bimap` | two-way map (the zonelayer state links characters and bodies with it) |
| `shared/channels` | `Drain` |
| `shared/logging` | key=value log lines; a call only fills a channel ([logging](logging.md)) |
| `shared/bitset` | K bits, the walls of a `TileMap` ([bitset](bitset.md)) |
| `shared/segmentlog` | append-only log in segment files, under the replay log ([segmentlog](segmentlog.md)) |
| `shared/binarycodec` | integers and strings as bytes, and `Version`, the version of the data that a `Decode` reads, for the replay log |
| `shared/clone` | the contract for copying: a value, or a type with `Clone`; the containers use it ([R29](../plan/rule/r29-clone-or-value.md)) |
| `shared/testing/aliasing` | for tests: finds memory that two values share, and checks that a type is a value or has `Clone` |
| `config` | `Var[T]` with defaults and limits; a snapshot taken once per tick; a whole set is rejected when one value is wrong |

## Temporary values

- The damage of the normal attack (1) and the regen (1) are written in the code; formulas replace them later.

## Replay

- Needs the inputs from outside and what only the time decides (`engine/replay/`); what engines send themselves is made again from the state.
- An engine records, in its own log: the messages it took (whole), the messages it sent that came back and the reason, the commands of clients, the version of the config it first read, the hash of its state every few ticks, and which copy of another engine it read first.
- In files: [replay_log](replay_log.md).
