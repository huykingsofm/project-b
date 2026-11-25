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
| The replay log writes one frame for each tick, says which tick is on the disk, and names its directory from the type of the state | `done` |
| A message carries the tick of its sender; what an engine takes or reads makes it depend on a tick of another engine (`StateDependency`) | `done` |
| The router says why a message has no engine: the engine was retired or has not started | `done` |
| An engine holds the messages it sent until the receiver says they are safe, and the log works out what is safe | `done` |
| A retired engine ends its log with a `Retired` frame, and its ID is never given again | `done` |
| The recovery line of a cluster, and the logs cut to it | `done` |
| Checkpoint files: each engine takes its own checkpoint every period from an offset of its own (the hash of its ID, as for the hash of the log), copied at the end of the tick and written by one writer for the cluster out of the goroutines of the engines; the log has the hash at the tick of each; all are kept for now (see [checkpoint](../../docs/checkpoint.md)) | `done` |

## To build

What is left of the recovery of a cluster: the logs can be cut to the recovery line and then replayed from tick 0 with no divergence, and every engine writes its checkpoints to files; what is not there yet is to start from them and to go on.

| Piece | What it is |
|---|---|
| Recovery line cache | the `Safe` of every engine, written to a file of its own by the goroutine that works out what is safe (a temporary file that is synced and renamed); a recovery reads the log of an engine from the tick after its cached one and not from the tick 1, after it checked that no cached tick is above the end of its log (if one is, the cache is not used and the logs are read whole); it is written again after a cut, and it is made with the replay from a checkpoint, since that is what makes the read of the logs from the tick 1 the slow part |
| Checkpoints above the line | the checkpoints of an engine whose tick is above its limit are dropped, since they hold what was cut |
| Replay from a checkpoint | each engine is made from its latest checkpoint at or below its limit and run on its own log to its limit, instead of from tick 0 |
| Handover | when every engine has run to its limit they go on live: the logs are written on after the cut, the unsettled messages are those of the checkpoints, and the acks are made again |
| Unsettled messages that are full | a limit for each destination, above which a new message bounces as `DeliveryFailureFull` and runs its `OnFail` |
| Logs of a cluster | all engines have a log or none has: a cluster that mixes them refuses to start |
| To measure | the size of the unsettled messages in a checkpoint; the time of a recovery, from tick 0 and from a checkpoint |

## Later

Status: `agreed`.

| Item | What it is |
|---|---|
| Engines made and removed while the game runs (priority: high) | today every engine is made once, by the builder, and `Cluster.Run` starts the ones it has when it starts; to make a zone or a zonelayer while running and to remove one: the world gives the ID (`lastStateID`, which counts the IDs of the zonelayers; after a recovery it is back at its value at the tick that was kept, so the log of an ID above it must be cut to nothing, or the making of an engine must depend on a tick of the world), with the IDs of zones that the content names kept apart from the ones given while running; the cluster adds and removes an engine safely while it runs (a lock, a goroutine for a new one, a retire for one removed); the world asks the cluster for it, since an action does not reach the cluster; replay and recovery know when an engine starts and ends (the `Retired` frame, and the ticks of an ID that go on from one life to the next) |
| Encoding and hash made asynchronous | `Append` keeps the struct, and a goroutine of the log encodes it and works out the hash from the copy of the state that the engine publishes and a copy it takes of what only it knows, in the order of the ticks; needs a rule that a struct given to `Append` is never changed (with a test), a bound on the list, and `engineHash` as a function of copies; measure the hash with a large zonelayer first |
| A version for each encoder | `Encode()` returns `(version, bytes)`, the version is written beside the bytes, and each `Decode` is given the version written for its own bytes; no one version for all the data; R32 changes with it |
| What a full ring lets go | today a ring (`shared/ring`: the chat of a zone and of a zonelayer, the private messages of the world) drops its oldest row when a new one does not fit, and a checkpoint writes every row it holds; to do: `Add` returns the row it lets go, and the state hands that row to a central store (the store of phase 6) instead of losing it; then what is past the ring lives in the store and not in the checkpoint |
| Encoder writers reused from a pool | `binary.Writer` is made as a literal at every place that encodes and its bytes are left to the collector; to do: `binary.NewWriter()` takes a writer from a `sync.Pool` with empty bytes, and `Recycle()` empties the bytes (keeping the capacity) and puts the writer back; every place that encodes calls `NewWriter` and then `defer w.Recycle()` at once; the bytes `B` are not exposed: `Len()` and `AppendTo(dst []byte) []byte` (an append, like `append`) are the only way out, so no slice of the pool's buffer can outlive `Recycle`, and what must live longer (what `Encode` returns, a frame handed to a log or a checkpoint) is appended to a buffer of the caller's; every use of `Writer{}` and of `.B` in code and tests is changed with it (pre-alpha, no shim); measure the allocations before and after |
| The configuration as an engine | today `config.Store` holds the versions and every engine reads a snapshot once for each tick, with the content of each version in the control stream; to do: a `ConfigEngine` that owns the configuration as its state (R28); the config store sends it an `UpdateConfig` command, which becomes an action of its own state; the other engines do not get a broadcast of actions (an action is of one type of state, and the configuration is not in their states) but read the copy it publishes with `StableRead`, with a `StateDependency` on its tick; every change is a command in its log and the snapshots are its periodic checkpoints, none for each change; when its log is not durable, the engines that read a tick beyond it go back to the recovery line as usual, and the config store sends again its latest version after a recovery, since the store is the source of the configuration; the engine ticks steadily, because every other engine depends on it; the control stream keeps only the engines added and removed |
