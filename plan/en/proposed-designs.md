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
- `ZoneLayerCharacterShieldGrantAction` in project-b is the first example of this kind (it blocks the first damage). The plan mechanism (`FindType`, `Cancel`, depth) is its foundation.

## Entities

An entity is an interface with `ID()`, `KindID()`, `Position()`, `Radius()` and `BoundingBox()`. Collision and pickup distance use the radius; `BoundingBox()` is derived from the position and the radius so that the grid knows which cells an entity is in. Every kind is a value type (R29), and every kind has its own table in the zonelayer state, keyed by `EntityID`, with its own events.

| Kind | Example data | Phase |
|---|---|---|
| Character (`CharacterEntity`) | `CharacterID`, stats, health, shield, cooldowns | 0 (the body exists) |
| Dropped item (`ItemEntity`) | item type ID, quantity, who may pick it up first (removed when its lifetime ends) | 0 |
| Corpse | the `CharacterID` that died, a list of items, expiry tick, whether it can be revived | 3 |
| Resource | type, amount left, refill tick, tool required | 3 |
| Portal | destination zone and spawn point, level requirement, enabled or not | 3 |
| Trap | owner, trigger radius, effect ID, ready tick, uses left, who can see it | 3 |
| Projectile | owner, direction and speed, damage, range left or expiry tick | 3 |

The first two kinds are built in phase 0; the others are added when needed, and each is pending approval. An action says which kinds it applies to through the general mechanism (R7), with no special case for each action.

An item has an ID of its own and exactly one owner (an entity on the ground, an inventory or a corpse). Inventories belong to the world, combat stats to the zonelayer (R28). Picking up: the zonelayer removes the item first and then sends a message to the world; if the message is lost, the item is lost but never duplicated, and the log can restore it.

## Replay log and checkpoint

Agreed (phase 1).

- **One log per engine.** Each engine writes only to its own file, with no shared lock; the cluster is put back in order by `tick` when replayed.
- **Two layers.** `shared/segmentlog` is a general append-only log: it knows segments, framing and the crash repair, and nothing of ticks or engines. The replay log puts what it needs in the two places the segment log leaves to its user, the meta of a segment header and the payload of a record.
- **Segments.** `<dir>/<scope>-<id>/seg-<index>.log` (for example `zonelayer-3/seg-00000000000000000000.log`), the index in 20 digits counted from 0 in the order the segments were made, closed when it reaches a configured size.
- **Record framing like Kafka's.** `[u32 length][u32 crc][payload]`, and a batch (a tick) ends with a commit marker, so a batch is all or nothing. After a crash, a half-written record or a batch with no commit at the end is detected through the length, the crc and the commit, and cut off (and kept next to the segment). The payload is the replay log's own: `[kind][tick][body]`, with the tick whole in each record so that a record does not depend on its segment.
- **Segment header.** Magic, format version and a meta. The replay log's meta is the format version of its records, the engine address, the seed, and the tick of the first record of the segment; a tick is found by reading the meta of about log2(n) segments, so no index file is needed.
- **Record bodies.** Commands and actions are written as numbers of registers (action ID, command ID, cooldown key); a command writes its own fields with its `Encode`, and a function in its file reads them, found by the ID in a table, so a new kind is new code and changes nothing that exists.
- **No cost during the tick.** The methods of `replay.Log` only append to a reused per-tick buffer. When the tick ends, the buffer goes to one writer goroutine that writes each batch with one `Write` and syncs at a configured interval (group commit), not on every tick.
- **Checkpoint.** `ckpt-<tick>.snap` (written to a temporary file, then renamed) encodes the copy the engine already published, outside the engine's goroutine. R29 guarantees the copy does not change. The log has a marker record at that tick; segments older than the oldest kept checkpoint are deleted (the number of kept checkpoints is a config parameter). The period is a config parameter, rarer than the hash, and offset per engine.
- **Recovery (option A).** Every engine loads its own latest checkpoint and replays its own input (messages, commands, config) to the end of the log. `StableRead` is not recorded by value, so after a failure the actions that read another engine's state run against the recovered states. A verifying replay of the whole cluster from tick 0 still compares the hash.
- **S3.** Pushing closed segments and checkpoints to S3 is in the plan at low priority (phase 6), not now.

## List of ideas

| Idea | What it must achieve in project-b | Phase | Status |
|---|---|---|---|
| Entity as an interface | `ID`, `KindID`, `Position`, `Radius`, `BoundingBox`; `CharacterEntity`, `ItemEntity`; a pickup action; appear and disappear events | 0 | detail pending approval |
| Checkpoint and replay log | The log is written to a file, append-only, in segments; periodic checkpoints offset between engines; recovery by option A | 1 | agreed |
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
| Write-behind queue | Writing to the store does not block a tick | 6 | pending approval |
| Character names | Unicode normalization, allowed alphabet, duplicate check, banned words | 6 | pending approval |
