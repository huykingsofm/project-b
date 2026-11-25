# Roadmap

What has been built and what is to be built, in the order it is done. What is built is described in `docs/`; what is to be built is described here until it is.

## Built

| Item | Status |
|---|---|
| Replay of a whole cluster from tick 0, on the real engines | `done` |
| Checkpoint of an engine, and an engine made from one | `done` |
| Fixes from a review of the replay and the checkpoint | `done` |
| The platform that runs states in ticks split from the game; the plan, the scheduler, the containers and the codecs rebuilt | `done` |
| IDs that a state gives out, counted up from 1 and never given again, from a number that the state holds (`lastStateID` of the world for its zonelayers, `lastEntityID` of a zone and a zonelayer for their entities) (see [ids](../../docs/ids.md)) | `done` |
| The event log writes one frame for each tick, says which tick is on the disk, and names its directory from the type of the state | `done` |
| A message carries the tick of its sender; what an engine takes or reads makes it depend on a tick of another engine (`StateDependency`) | `done` |
| The router says why a message has no engine: the engine was retired or has not started | `done` |
| An engine holds the messages it sent until the receiver says they are safe, and the log works out what is safe | `done` |
| A retired engine ends its log with a `Retired` frame, and its ID is never given again | `done` |
| The recovery line of a cluster, and the logs cut to it | `done` |
| Checkpoint files: each engine takes its own checkpoint every period from an offset of its own (the hash of its ID, as for the hash of the log), copied at the end of the tick and written by one writer for the cluster out of the goroutines of the engines; the log has the hash at the tick of each; all are kept for now (see [checkpoint](../../docs/checkpoint.md)) | `done` |
| Recovery line cache: the `Safe` of every engine, written to a file of its own by the observer of the event log every `replay.line_cache_every_ticks` and when the log is closed; a recovery reads each log from the tick after its cached one, and reads it whole when the cache says more than a log holds (see [replay](../../docs/replay.md)) | `done` |
| Checkpoints above the line: moved, with what is cut of the logs, to the archive of the cut (`.archive/cut-<n>/`), and never deleted (see [replay](../../docs/replay.md#cut)) | `done` |
| Replay from a checkpoint: each engine is made from its latest checkpoint at or below its limit (an earlier one where another engine reads a copy of it before) and run on its own log to its limit, with the hash of the log at the checkpoint compared with the engine that was made (see [replay](../../docs/replay.md#start-of-an-engine)) | `done` |
| `platform/eventlog` keeps the log of an engine, and `platform/replay` brings a cluster back: the recovery line and the start of each engine make a `Plan` (read-only), `Plan.Cut` brings the files to it, and `Run` runs the engines on it, so a replay only runs on logs that are whole at the recovery line; `Recover` is the three, and an engine that differs from its log is a `*DivergenceError` (see [replay](../../docs/replay.md)) | `done` |

## To build

What is left of the recovery of a cluster: the logs and the checkpoints can be cut to the recovery line and every engine replayed from its checkpoint to its limit with no divergence; what is not there yet is to go on live from there.

| Piece | What it is |
|---|---|
| Handover | when every engine has run to its limit they go on live: the logs are written on after the cut, the unsettled messages are those of the checkpoints, and the acks are made again |
| Unsettled messages that are full | a limit for each destination, above which a new message bounces as `DeliveryFailureFull` and runs its `OnFail` |
| Logs of a cluster | all engines have a log or none has: a cluster that mixes them refuses to start |
| To measure | the size of the unsettled messages in a checkpoint |

## Later

Status: `agreed`.

| Item | What it is |
|---|---|
| Engines made and removed while the game runs (priority: high) | today every engine is made once, by the builder, and `Cluster.Run` starts the ones it has when it starts; to make a zone or a zonelayer while running and to remove one: the world gives the ID (`lastStateID`, which counts the IDs of the zonelayers; after a recovery it is back at its value at the tick that was kept, so the log of an ID above it must be cut to nothing, or the making of an engine must depend on a tick of the world), with the IDs of zones that the content names kept apart from the ones given while running; the cluster adds and removes an engine safely while it runs (a lock, a goroutine for a new one, a retire for one removed); the world asks the cluster for it, since an action does not reach the cluster; replay and recovery know when an engine starts and ends (the `Retired` frame, and the ticks of an ID that go on from one life to the next) |
| Encoding and hash made asynchronous | `Append` keeps the struct, and a goroutine of the log encodes it and works out the hash from the copy of the state that the engine publishes and a copy it takes of what only it knows, in the order of the ticks; needs a rule that a struct given to `Append` is never changed (with a test), a bound on the list, and `engineHash` as a function of copies; measure the hash with a large zonelayer first |
| A version for each encoder | `Encode()` returns `(version, bytes)`, the version is written beside the bytes, and each `Decode` is given the version written for its own bytes; no one version for all the data; R32 changes with it |
| What a full ring lets go | today a ring (`shared/ring`: the chat of a zone and of a zonelayer, the private messages of the world) drops its oldest row when a new one does not fit, and a checkpoint writes every row it holds; to do: `Add` returns the row it lets go, and the state hands that row to a central store (the store of phase 6) instead of losing it; then what is past the ring lives in the store and not in the checkpoint |
| Encoder writers reused from a pool | `binary.Writer` is made as a literal at every place that encodes and its bytes are left to the collector; to do: `binary.NewWriter()` takes a writer from a `sync.Pool` with empty bytes, and `Recycle()` empties the bytes (keeping the capacity) and puts the writer back; every place that encodes calls `NewWriter` and then `defer w.Recycle()` at once; the bytes `B` are not exposed: `Len()` and `AppendTo(dst []byte) []byte` (an append, like `append`) are the only way out, so no slice of the pool's buffer can outlive `Recycle`, and what must live longer (what `Encode` returns, a frame handed to a log or a checkpoint) is appended to a buffer of the caller's; every use of `Writer{}` and of `.B` in code and tests is changed with it (pre-alpha, no shim); measure the allocations before and after |
