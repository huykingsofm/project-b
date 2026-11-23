# Roadmap

What has been built and what is to be built, in the order it is done. What is built is described in `docs/`; what is to be built is described here until it is.

## Built

| Item | Status |
|---|---|
| Replay of a whole cluster from tick 0, on the real engines | `done` |
| Checkpoint of an engine, and an engine made from one | `done` |
| Fixes from a review of the replay and the checkpoint | `done` |
| The platform that runs states in ticks split from the game; the plan, the scheduler, the containers and the codecs rebuilt | `done` |
| IDs that a state gives out, with a grave: `shared/ids`; the entities of a zonelayer and a zone, and the zonelayers of the world, use it (see [ids](../../docs/ids.md)) | `done` |

## To build

One pull request, on top of the one that gives out IDs, in two parts that are commits of it.

### Part 1: replay log

Status: `agreed`.

| Piece | What it is |
|---|---|
| Type names | `StateType.String()` reads a registry filled by `MapStateTypeToName`; a type has one name, never a duplicate, never changed; `Describe` goes, and the directory of a log comes from the state ID |
| `Wants` | goes: a log writes what it is given, and `NopLog` is a place holder that takes everything |
| Access | a type reaches into another only through its methods, named after the field (`Pending()`, `SetPending()`) |
| Frame | one frame for each tick, even an empty one: `[tick][record...]`, a record is `[len\|kind\|data]`; `Append` encodes into the buffer of the tick; `EndTick` works out the hash (every `every` ticks), writes the frame as one record of the segment log and commits |
| Segment log | a reader that reads from bytes (`SegmentReader`), and a count of the batches that were synced |
| Durable tick | the log says which tick of an engine is synced |

### Part 2: recovery of a cluster

Status: `agreed`.

| Piece | What it is |
|---|---|
| Tick in the header | a message carries the tick of its sender (`Header.Tick`) |
| State dependency | one struct, `StateDependency{Source, Tick}`, made by an `Inbound` (from its header) and by a `StableRead` (`Source`, `CopyTick`; none when it is 0) |
| Reasons of a failed send | a status of its own for an ID never given, treated like a destroyed one by an action; `NotBuilt` renamed `NotStarted`; `Retry`, `Redirect` and `Escalate` drop at once for a destination that is gone, and the `OnFail` runs once |
| Outbox | one structure for what the bouncer holds and what was sent and not yet acked (`sim.Outbox` stays apart); it is in the checkpoint; resend after 1, 2, 3 ... ticks up to a most (numbers in the configuration, not in the checkpoint); the limit for each destination bounces a new message as `Full` |
| Safe | the log works out `safe` for each engine, again after each sync: a tick is safe when it is durable and every dependency of it and of the ticks before is at a safe tick of its source; an engine that is destroyed is still followed |
| Ack | not a message: `ackSeq` for each sender, the `lastSeq` of the receiver at its safe tick, made again at a restore, read by the sender when it flushes; a copy that was taken before is not acked by being dropped |
| Retire | an engine that is destroyed writes a frame that holds `Retired`; an ID that is given again goes on counting ticks after the last tick of its last life, so a tick of an ID is never the tick of two lives; the allocator keeps that last tick for each ID |
| Recovery line | each engine is cut before the first tick with a dependency above the durable tick of its source, until nothing changes; then replayed once from its latest checkpoint at or below its limit; a checkpoint above is dropped; all checkpoints are kept for now |
| Cut | `CutAfter(tick)` on a log, called with the limit that recovery worked out; a cut loses the whole tick, commands included; the hash is checked only up to the limit |
| Logs of a cluster | all engines have a log or none has |
| To measure | the size of the outbox in a checkpoint; the time of a recovery |

## Later

Status: `agreed`.

| Item | What it is |
|---|---|
| Encoding and hash made asynchronous | `Append` keeps the struct, and a goroutine of the log encodes it and works out the hash from the copy of the state that the engine publishes and a copy it takes of what only it knows, in the order of the ticks; needs a rule that a struct given to `Append` is never changed (with a test), a bound on the list, and `engineHash` as a function of copies; measure the hash with a large zonelayer first |
| A version for each encoder | `Encode()` returns `(version, bytes)`, the version is written beside the bytes, and each `Decode` is given the version written for its own bytes; no one version for all the data; R32 changes with it |
