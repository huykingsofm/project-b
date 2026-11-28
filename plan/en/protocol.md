# Protocol between the client and the server

Status: proposed, for the owner to choose in the last section. No file of `proto/` is written before. The protocol belongs to phase 2 ([phases.md](phases.md)); the parts around it (the gateway, the delivery of events, the client) are in [proposed-designs.md](proposed-designs.md#early-playable-slice-phase-2).

## What is decided already, and what the protocol must be

- Protobuf with `buf`, binary frames over WebSocket, events batched per tick, code generated for Go and TypeScript ([decisions.md](decisions.md)).
- Fast and light: a client sends intents, never positions or results; a tick of an engine is one frame to a viewer; nothing a viewer may not see is on the wire (the no-leak test of phase 3).
- It is not frozen before the first release: a breaking change is made as it is, and `buf breaking` is not run ([R15](../rule/r15-versioned-data-formats.md)).
- It is generic where the platform is, and the game's where the game is ([R2](../rule/r02-business-in-own-layer.md)): the gateway must not know what a monster or an item is.

## Two layers

| Layer | Package | Knows | Changes when |
|---|---|---|---|
| Transport | `transport.v1` | a connection: hello, welcome, ping, a batch of what happened in one engine at one tick, a refusal, a close | the platform changes |
| Game | `game.v1` | this game: the commands of a player, and the events and the snapshot a player is told | the game changes |

A game message goes inside a transport frame as `bytes` (the serialized `game.v1.ClientMessage` or `ServerMessage`). So the gateway carries and batches without knowing the game, and the adapter of the game is the only code that makes and reads game messages. An event is serialized once and the same bytes go to every viewer of its channel: the response of a channel is built once ([decisions.md](decisions.md)). A new feature of the game adds a case to a `oneof` of `game.v1` and touches nothing of `transport.v1`.

## The connection

| Step | What happens |
|---|---|
| 1 | the client opens a WebSocket (`wss` in production); the server waits for the first frame for a short time and closes the connection when none comes |
| 2 | the client sends `Hello`: the protocol version, the name of the client build, and one of two credentials: a guest (a name) or a token |
| 3 | the server answers `Welcome`: the version, the length of a tick, the limits of the connection, and a token to come back with; or closes with a code (below). **This is the only authentication:** a later frame carries no credential |
| 4 | the client sends `EnterWorld`; the server answers with a `Snapshot` and from then on a batch for each tick of each engine that the client watches |
| 5 | `Ping` and `Pong` carry the time of the client back, for the round trip; a connection that sends nothing for two periods of the heartbeat is closed |

Every frame is one WebSocket binary message, at most the size that `Welcome` says. Compression of the message is off: a batch is small, and the cost of the compression is paid on the goroutine of the gateway for every connection.

| Close code | Meaning |
|---|---|
| 1000 | normal |
| 4000 | the frame is not a valid frame, or arrived in the wrong state |
| 4001 | the version is not the one of the server (the answer carries the version it wants) |
| 4002 | the credentials are refused |
| 4003 | the connection does not read and its queue is full |
| 4004 | too many commands |
| 4005 | the server is stopping |
| 4006 | the account was entered somewhere else |

## What a client says

A command is an intent. The acting character is never in it: the session holds the character, and the adapter puts it in the command of the simulation, so a client cannot act as another character. Each command has `seq`, a number that the client counts up, which comes back in a refusal.

| Message | Fields | Becomes |
|---|---|---|
| `EnterWorld` | the name of the character | a character made for the session and put in the first zone ([proposed-designs.md](proposed-designs.md#early-playable-slice-phase-2)) |
| `Move` | `dir` (one of eight), `ticks` | `ZoneLayerCharacterMoveCommand`: the character runs in that direction for that many ticks, at its speed; the server decides where it ends |
| `Attack` | the character that is the target | `ZoneLayerCharacterNormalAttackTriggerCommand` |
| `PickUp` | the entity ID of the item | `ZoneLayerCharacterItemPickupCommand` |
| `TakeExit` | the entity ID of the exit | `ZoneLayerCharacterExitToCommand` |
| `Chat` | the scope (zonelayer or zone) and the text | `ZoneLayerChatSendCommand` or `ZoneChatSendCommand` |

The abilities of the game (the shield is the only one built, as a sample) are not in this table: each will be a case of its own in `game.v1` when the game makes it, and none is on the wire before.

## What the server tells

**One frame for one tick of one engine.** A viewer may watch several engines (its zonelayer, its zone, the world), and they tick at their own pace, so there is no clock for the whole connection. A frame `EventBatch` holds: the engine, its kind (world, zone or zonelayer), the tick, `engine_seq` (the number of the batch among the batches of that engine), and the events of that tick that the viewer may see, each as bytes.

| Event | Fields | Comes from |
|---|---|---|
| `EntityAppeared` | entity ID, kind, position, radius, and for a character its ID, name, health, and for an item its ID and count | `CharacterEntered`, `ItemAppeared` |
| `EntityDisappeared` | entity ID, where it was | `CharacterLeft`, `ItemDisappeared` |
| `EntityMoved` | entity ID, to, and where it was | `EntityMoved` |
| `HealthChanged` | entity ID, the change, the health now | `EntityHealthChanged` |
| `EntityDied` | entity ID, where | `EntityDied` |
| `ChatMessage` | the scope, the character that said it, the text, the tick | `ChatMessageSent` of a zonelayer or a zone |
| `ItemsReceived` | the item, how many (private to the character) | `CharacterItemAdded` |
| `ExitAdded` | entity ID, the area, the zone it leads to, where it arrives | `ExitAdded` |
| `CommandRejected` | the `seq` of the command, a reason | the gateway, for a command that is too large, too fast or not allowed (a refusal by the simulation is told when the actions are given a way to say it) |

- **Units:** positions and radii are in units, `sint32` (a tile is 1,024 units); health and its changes are `int32`; IDs are `uint64`; a direction is an enum of eight and none. Nothing is a float, so the client and the server cannot differ by rounding.
- **A move may be told more than once for an entity in a tick**, and the protocol does not care: one `EntityMoved` for each entity a tick, with the first `from` and the last `to`, is an option of [decisions.md](decisions.md) that the delivery can take later without a change of the wire.
- **The audience is not on the wire.** The server uses it to choose the channels of a frame, and a client is sent only what it may see.

## Entering, and the snapshot: the method that loses no event

The snapshot is the published copy of an engine at a tick `T`: for the zonelayer, the map, the entities (with the fields of `EntityAppeared`), the exits and the character of the session. The rule of the client is: **after a snapshot of an engine at `T`, only the batches of that engine with a tick above `T` count.** What makes the rule true, and loses nothing:

1. **The order of the engine** (decided): at the end of a tick `t`, `Engine.Step` publishes the copy of `t` first and emits the events of `t` after. Today it emits first; this is a change of the platform ([roadmap.md](roadmap.md)).
2. **The adapter that is the `Emitter`** gives each batch of an engine a number `engine_seq`, one more than the batch before it of the same engine, and keeps for each engine a list of subscribers, each with a bounded queue. `Emit` numbers the batch and appends it to every queue, under a lock.
3. **To enter,** for each engine the session watches:
   - subscribe: a queue is made that will get every batch emitted from now on;
   - then read the latest published copy of the engine, at the tick `T`;
   - build the snapshot with the mark `(engine, T)` and send it;
   - then send the queue, leaving out the batches with a tick not above `T`.
4. **Why no batch is missed.** A batch of a tick `t` above `T` is emitted after the copy of `t` was published (the order above). That publish came after the read, because a read made after it would have returned `t` or later, not `T`. And the read came after the subscription. So the batch is emitted after the subscription, and it is in the queue. A batch with a tick up to `T` may be in the queue too (the events of `T` are emitted after the copy of `T`); it is left out, since the copy already holds what it did.
5. **The engine keeps no event.** The only memory of events is the queue of each subscriber. When a queue is full (a client that does not read), the connection is closed (4003) and the client enters again with a new snapshot: no batch is skipped without the client knowing.
6. **A gap is found, not guessed.** An engine emits no batch for a tick that has no events, so the ticks of two batches are not one apart. The client checks `engine_seq` instead: in the batches of an engine it goes up by exactly one. A jump means a batch was lost, and the client enters again.
7. **A mark for each engine.** A session watches more than one engine (the zonelayer, its zone for the exits and the chat of the zone, the world for what is private to the character), and each has its own clock, so the snapshot carries a mark `(engine, tick)` for each, and the rule of the client is applied to each engine alone.

**Why not keep the events of a tick until a copy of a higher tick is published.** A reader that read the copy `T` and subscribes later would find the batches above `T` in that store, but only while they are kept: a reader that is slower than one tick finds the batch of `T+1` gone, because the copy of `T+2` was published. To make that sure the store would need a lease for each reader (the events kept until each reader says it has them), which is the queue of the subscriber, kept in the engine instead of the adapter. Subscribing first needs no store in the engine and no lease.

A session that goes to another zonelayer (an exit) enters the zonelayer that it arrives in by the same steps, and its frames of the old engine stop.

## The map

`TileMap` never changes. The snapshot carries its size in tiles and its walls as one bit for each tile (200 by 200 tiles is 5 KB, 1,000 by 1,000 is 125 KB); the client keeps it for the zone. Where the map is a file the client fetches (phase 3), the snapshot carries the name and the hash of the map instead; the choice is below.

## Time and the move of the client

The client applies the move rule itself and moves at once, as [R8](../rule/r08-player-first.md) says; the server sends the position that is real in `EntityMoved`, and the client corrects when its own character is not where the server says. For this the client needs only the `from` and the `to` of each move of its entity, and the tick of the frame; it interpolates the others between the ticks it receives. The length of a tick is in `Welcome`. The move rule of the client is tested against vectors that Go makes from `TileMap.Reach`.

## Size and speed

- A frame `EventBatch` of one tick is the sum of its events. An `EntityMoved` is about 15 bytes; with 150 entities that move, about 2.3 KB for a tick and a viewer, before the grid of phase 3 takes it down. This is an estimate, to be measured with the bots.
- The limits are config variables sent in `Welcome`: the largest frame, the commands for each tick, the queue of a connection, the length of a chat text.
- A viewer that cannot keep up is closed (4003), never slowed: the engine only appends to a bounded queue.

## The draft of the files

The two drafts compile with `buf build` and pass `buf lint` with the `BASIC` rules.

`proto/transport/v1/transport.proto`:

```proto
syntax = "proto3";
package transport.v1;

message ClientFrame {
  oneof body {
    Hello hello = 1;
    Ping ping = 2;
    bytes game = 3; // a game.v1.ClientMessage
  }
}

message ServerFrame {
  oneof body {
    Welcome welcome = 1;
    Pong pong = 2;
    EventBatch batch = 3;
    bytes game = 4; // a game.v1.ServerMessage that is not an event: the snapshot
    Refusal refusal = 5;
  }
}

message Hello {
  uint32 protocol_version = 1;
  string client = 2;
  oneof credentials {
    Guest guest = 3;
    string token = 4;
  }
}

message Guest { string name = 1; }

message Welcome {
  uint32 protocol_version = 1;
  uint32 tick_period_ms = 2;
  string token = 3;
  Limits limits = 4;
}

message Limits {
  uint32 max_frame_bytes = 1;
  uint32 max_commands_per_tick = 2;
  uint32 max_text_bytes = 3;
}

message Ping { uint64 client_time_ms = 1; }
message Pong { uint64 client_time_ms = 1; uint64 server_time_ms = 2; }

enum Scope { SCOPE_UNSPECIFIED = 0; SCOPE_WORLD = 1; SCOPE_ZONE = 2; SCOPE_ZONELAYER = 3; }

message EventBatch {
  uint64 engine_id = 1;
  Scope scope = 2;
  uint64 tick = 3;
  uint64 engine_seq = 4;        // goes up by one in the batches of this engine
  repeated bytes events = 5;    // each a game.v1.ServerMessage, serialized once
}

message Refusal {
  uint32 client_seq = 1;
  string text_key = 2;          // for the localization of phase 7
  uint32 code = 3;
}
```

`proto/game/v1/game.proto`:

```proto
syntax = "proto3";
package game.v1;

message Vec { sint32 x = 1; sint32 y = 2; }

enum Dir {
  DIR_NONE = 0; DIR_NORTH = 1; DIR_NORTH_EAST = 2; DIR_EAST = 3; DIR_SOUTH_EAST = 4;
  DIR_SOUTH = 5; DIR_SOUTH_WEST = 6; DIR_WEST = 7; DIR_NORTH_WEST = 8;
}

enum EntityKind { ENTITY_KIND_UNSPECIFIED = 0; ENTITY_KIND_CHARACTER = 1; ENTITY_KIND_ITEM = 2; ENTITY_KIND_EXIT = 3; }
enum ChatScope { CHAT_SCOPE_UNSPECIFIED = 0; CHAT_SCOPE_ZONELAYER = 1; CHAT_SCOPE_ZONE = 2; }

message ClientMessage {
  uint32 seq = 1;
  oneof body {
    EnterWorld enter_world = 2;
    Move move = 3;
    Attack attack = 4;
    PickUp pick_up = 5;
    TakeExit take_exit = 6;
    Chat chat = 7;
  }
}

message EnterWorld { string name = 1; }
message Move { Dir dir = 1; uint32 ticks = 2; }
message Attack { uint64 target_character_id = 1; }
message PickUp { uint64 item_entity_id = 1; }
message TakeExit { uint64 exit_entity_id = 1; }
message Chat { ChatScope scope = 1; string text = 2; }

message ServerMessage {
  oneof body {
    Snapshot snapshot = 1;
    EntityAppeared entity_appeared = 2;
    EntityDisappeared entity_disappeared = 3;
    EntityMoved entity_moved = 4;
    HealthChanged health_changed = 5;
    EntityDied entity_died = 6;
    ChatMessage chat = 7;
    ItemsReceived items_received = 8;
    ExitAdded exit_added = 9;
  }
}

message Entity {
  uint64 entity_id = 1;
  EntityKind kind = 2;
  Vec pos = 3;
  sint32 radius = 4;
  oneof detail { CharacterDetail character = 5; ItemDetail item = 6; }
}

message CharacterDetail { uint64 character_id = 1; string name = 2; int32 health = 3; int32 max_health = 4; }
message ItemDetail { uint32 item_id = 1; uint32 count = 2; }

message EngineMark {
  uint64 engine_id = 1;
  uint64 tick = 2;              // only the batches of this engine above this tick count
  uint64 engine_seq = 3;        // the number of the first batch the client is sent
}

message Snapshot {
  repeated EngineMark marks = 1;
  uint64 zone_id = 2;
  uint64 zonelayer_id = 3;
  uint64 self_character_id = 4;
  uint64 self_entity_id = 5;
  uint32 width_tiles = 6;
  uint32 height_tiles = 7;
  bytes walls = 8;               // one bit for each tile, row by row
  repeated Entity entities = 9;
}

message EntityAppeared { Entity entity = 1; }
message EntityDisappeared { uint64 entity_id = 1; Vec at = 2; }
message EntityMoved { uint64 entity_id = 1; Vec from = 2; Vec to = 3; }
message HealthChanged { uint64 entity_id = 1; int32 change = 2; int32 health = 3; }
message EntityDied { uint64 entity_id = 1; Vec at = 2; }
message ChatMessage { ChatScope scope = 1; uint64 from_character_id = 2; string text = 3; uint64 tick = 4; }
message ItemsReceived { uint32 item_id = 1; uint32 count = 2; }
message ExitAdded { uint64 entity_id = 1; Vec min = 2; Vec max = 3; uint64 to_zone_id = 4; Vec arrival = 5; }
```

## What it needs from the simulation (the owner decides, [R5](../rule/r05-pattern-exemplar.md))

1. **Read methods of `zonelayer.State` for the snapshot:** its entities (kind, position, radius, and for a character its ID, name, health and maximum), its zone and its map. Today the read-only interface of a zonelayer has only `CharacterCount`.
2. **How a guest becomes a character** and which zone it starts in: the command `EnterWorld` becomes a command of the world or of a zone that makes the `Character` with its defaults (a sample, R2).
3. **The order of the end of a tick:** `Engine.Step` publishes the copy of the tick, then emits its events (decided; today it is the other way). Without it the method of the snapshot has a gap.
4. **A way for an action to refuse in a way a session can be told:** an action does not fail ([R10](../rule/r10-errors.md)) but may say a refusal through the state; the protocol has `CommandRejected` for the gateway's own refusals, and the refusals of the simulation come when an event for them exists.

## Choices of the owner

| Choice | Options | Proposed |
|---|---|---|
| Where the game's bytes go | typed `oneof` of the game inside the transport frame (the gateway imports the game); or `bytes` that the adapter fills | `bytes`: the gateway stays generic and an event is serialized once for every viewer |
| One frame for what | one frame for each tick of each engine; or one frame for each tick of the connection with a list of engines | one for each tick of each engine: it follows the channels, and no clock is invented |
| Ack of a command | none, the events show the result; or an `ack` with the last `seq` that the engine took | none now; add the `ack` if the correction of the client needs it, which needs the engine to report it |
| The map in the snapshot | the walls inline; or the name and hash of a file the client fetches | inline for phase 2 (small maps), the file with the map reader of phase 3 |
| Reconnect | a new connection is a new enter; or the token brings the session back for a time | a new enter in phase 2; the token is in `Welcome` so the wire does not change in phase 7 |
| Generated code | committed to the repository, with a check that it is up to date; or generated by the build | committed (`go build` needs no `buf`), checked by `make proto` |
| Names of the packages and files | `proto/transport/v1`, `proto/game/v1`; or one package | the two layers, as above |
