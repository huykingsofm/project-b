# Checkpoint of an engine

`platform/runtime/engine/checkpoint.go` (with `CheckpointWriter`), the period in `platform/runtime/engine/engine.go`, the files in `platform/journal/checkpoint`, with the codecs of the states (`Encode` in `server/game/sim/state/*/state.go`, the decoder in `server/game/sim/state/*/codec.go`), of the entities (`server/game/sim/state/zonelayer/entity/codec.go` and `server/game/sim/state/zone/entity/codec.go`) and of the scheduler (`platform/sim/scheduler.go`). What an engine needs to go on from the tick it was taken at: `engine.Checkpoint[S]`, which holds it as data that shares nothing with the engine, and its bytes. The design: `plan/en/proposed-designs.md`.

## Entry points

| Call | What it does |
|---|---|
| `e.Checkpoint()` | takes the checkpoint of an engine as it is: a clone of its state, its scheduler, its random numbers and what its peer knows; called between two ticks, by the goroutine of the engine or while it does not run |
| `c.Encode()` | writes a checkpoint as bytes (below) |
| `engine.DecodeCheckpoint[S](data, codecs)` | reads a checkpoint of an engine whose state is of type `S`; the state is read with the codecs (the state of a zone holds its map); a checkpoint of an engine whose state is of another type is an error |
| `c.Restore(env)` | makes the engine of a checkpoint, at its tick; the engine takes what the checkpoint holds; its ID is the one in the header, and the seed comes from `env` |
| `checkpoint.LatestAtOrBelow(dir, id, tick)` | the tick of the latest checkpoint file of an engine at or below `tick`, and false when there is none |
| `checkpoint.Archive(dir, id, tick, archiveDir)` | moves the checkpoint file of an engine at a tick to `archiveDir`: it is no longer a checkpoint, and is not deleted; the file is named `ckpt-<tick>.<now>.snap` there (`atomicfile.Stamped`), so that two moves never meet |
| `replay.CheckpointSource` | which checkpoint each engine of a replay starts from (`Checkpoint(id)`); the files under the directory of the logs are the one of a recovery |
| `engine.EngineRegistry.Restore(env, data)` | makes the engine of a checkpoint without knowing its type: the type is in the ID of the header |
| `checkpoint.NewFileWriter(dir, queue)` | the `engine.CheckpointWriter` of a cluster: one goroutine that writes the checkpoints of every engine to files under `dir`, with a queue of `queue`; `Close` writes what waits and stops it |
| `checkpoint.Ticks(dir, id)` | the ticks at which an engine has a checkpoint file, in rising order |
| `checkpoint.Read(dir, id, tick)` | the bytes of the checkpoint of an engine at a tick |

- `Restore` registers the engine in the router of `env`, as `engine.New` does; a checkpoint whose state is not at its tick is an error, and registers nothing.
- What an engine does again and again (a zonelayer healing its characters) is an action that sets itself again, so it waits in the scheduler and is in the checkpoint like any other. An engine that a request made is given its first one as an action it starts with (`WorldZoneLayerAddAction` gives the regen of a zonelayer when `RegenEvery` is above 0).
- A checkpoint that is cut, has a byte too many, is of another data version or of another type of state is an error, never a panic.

## What it holds

| Field of `Checkpoint` | Holds |
|---|---|
| `ID` | the ID of the engine |
| `Tick` | the tick the engine is at |
| `State` | the state, a copy that no one changes |
| `ActionScheduler` | what the engine waits for, each action a clone |
| `Rand` | the random numbers of the engine, a clone that goes on from where they are |
| `Peer` | the number of the last message the engine sent, the last taken from each sender, and the unsettled messages, each a clone |

## What is written

In this order:

| Part | Holds |
|---|---|
| version | the version of the data, `uvarint` (1) |
| state ID | the ID of the engine, `uvarint`; its high 8 bits are the type of the state, which the restore checks against the type it was asked for, and the ID against the state |
| tick | the tick the engine is at |
| state | the data of the state, by its `Encode` (below) |
| scheduler | the last tick it gave out, and the actions set for the ticks to come (the next one included), by tick and in order |
| random numbers | where the generator of the engine is (the PCG), as its own bytes |
| `sentSeq` | the number of the last message the engine sent |
| `lastRecvSeq` | the last number taken from each sender, in the order of the state IDs |
| unsettled messages | the messages that are to go, and the messages that went and were not acked, each written whole ([unsettled messages](unsettled_messages.md)) |

## What is not written

| Left out | Why |
|---|---|
| the events of the state | they are not data (R9); the engine gives them to its clients at the end of the tick |
| the map of a zone | it never changes, and the ID of the zone finds it |
| the queues of the engine | what was not yet taken from them is in the event log |
| `View`, the registry of the tick, the emitters | made again by the engine |

## State, by type

| State | Data written |
|---|---|
| world | ID, tick, every zone and zonelayer, the IDs it gives its zonelayers (see [ids](ids.md)), the private messages, the inventory of each character |
| zone | ID, world ID, tick, its entities (the exits, each with the zone it leads to), the zonelayers it knows with their capacity, what was said, the IDs it gives its entities (see [ids](ids.md)) |
| zonelayer | IDs (zonelayer, zone, world), tick, its entities, its characters with their cooldowns, what was said, which character has which body, the IDs it gives its entities (see [ids](ids.md)) |

- An entity of a zonelayer is written as its kind (`KindID`) and then what it is made of; each kind has its decoder in the `codec.go` of the `entity` package of its state, and a test fails when a kind of the register has none. An entity writes only the number its state gave it, not the whole `EntityID`: the state is the owner of all its entities and gives its own ID back when it reads them (the bodies of the characters are written the same way), and a state that holds an entity of another owner cannot be written. An entity is written once, not under a key.
- A count that claims more items than the bytes left can hold is refused before anything is made.

## Hash

- The hash of an engine covers what the checkpoint of it holds, which is what the engine goes on from:

| Part of the hash | How |
|---|---|
| state | the hash of the state |
| scheduler, self list | each action as the bytes it is written as, so a field only the action knows (a shield that started) changes it |
| what only the engine knows | the bytes the checkpoint writes for it: random numbers |
| what its peer knows | the bytes the checkpoint writes for it: `sentSeq`, `lastRecvSeq`, the messages that are to go |

- A restore that differs from the engine it came from in any of them has another hash, and a replay finds it.

## Taken at the end of a tick

Each engine takes its own checkpoints, with no set for the cluster: no checkpoint of one engine depends on another.

| Key | Value |
|---|---|
| when | at every tick for which `(tick + offset mod every) mod every` is 0 |
| `every` | the config variable `checkpoint.every_ticks` (600) |
| offset | the hash of the ID of the engine, the same offset as for the hash of the event log, so the engines of a cluster are spread over the ticks of a period |
| where in `Step` | after the messages of the tick are sent, before `EndTick` |
| on the goroutine of the engine | a `Checkpoint` whose state is the clone that was just published, which no one changes; the scheduler, the random numbers and what the peer knows are cloned |
| on the goroutine of the writer | the checkpoint is encoded (`Encode`), and the file is written |
| the hash | the event log has the hash of the engine at every tick a checkpoint is taken at, whether the hash is due or not; a replay, whose engines take no checkpoint, hashes at the same ticks, since it depends on the configuration and the tick alone |
| no writer in `Environment` | no checkpoint is taken (a replay) |

## Files

| Key | Value |
|---|---|
| path | `<dir>/<name>-<id>/ckpt-<tick>.snap`, in the directory of the engine next to its event log (`eventlog.EngineDir`); the tick in 20 digits |
| how it is written | with `atomicfile.Write` ([atomicfile](shared.md#atomicfile)): to `ckpt-<tick>.snap.tmp`, synced, renamed, then the directory is synced: a file with the name of a checkpoint is whole |
| a temporary file left by a crash | not a checkpoint; written over when the same tick is written again |
| order | one writer for the cluster, in the order the checkpoints were offered |
| a full queue, or a writer that is closed | the checkpoint is dropped, logged and counted (`Dropped`); the engine goes on, and its next checkpoint is a period later |
| a checkpoint that cannot be encoded or written | logged and counted (`Failed`); no file is left |
| deleted | never: all are kept for now; the ones above the recovery line go to the archive of the cut ([replay](replay.md#cut)) |

## Starting from a checkpoint

- A replay or a recovery makes each engine from its latest checkpoint at or below its limit, and runs it on its log: [replay](replay.md#an-engine-that-starts-from-a-checkpoint).
- The engine made from a checkpoint has the hash that the log recorded at the tick of the checkpoint, which the replay checks.
- `Result.Handover` makes the engines that go on live after it, each from the checkpoint of its replayed engine at its limit ([replay](replay.md#handover)).

Benchmarks: [checkpoint](benchmark/checkpoint.md).
