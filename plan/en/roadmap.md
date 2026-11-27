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
| Handover: `Result.Handover(env, store)` after `Recover` makes the engines again in the environment of a cluster that runs (`engine.NewEnvironment`), each from a checkpoint of its replayed engine at its limit; the replay has an `AckBoard`, so the checkpoint of each engine holds the messages it sent that no receiver took, and the first flush of a live engine sends them again and a receiver that took one drops the copy; a message that came back is let go of from the sent ones at once (`handleReturnedMessages`), so its `OnFail` runs once; an engine that the replay removed is not made again; the event log writes on after the cut (see [replay](../../docs/replay.md#handover)) | `done` |
| Making and removing engines while the game runs, by directives: an action asks (`Registry.RequestCreateEngine(tag, state, startActions...)`, `RemoveEngine(id, tag)`); the engine numbers each request (`sim.DirectiveRequest`), keeps it in its own data (checkpoint and hash) and hands it to its cluster (`DirectiveSink`) before it sends the messages of the tick, and again at its first tick and every `directive.resend_every_ticks` until the answer is taken; the cluster gives the ID (`Cluster.NewStateID`; it counts above what its logs hold), makes the engine from the state and the actions it starts with by the `engine.EngineRegistry` (one line for each type of state, in `codecs.go`), serves a request once (`(engine, request ID)` keys the engines it made) and answers the engine that asked with a `sim.DirectiveResponse`; the engine takes the answers first in its tick, writes each to its log (`eventlog.Directive`), and its state takes it (`sim.DirectiveHandler`); a removal ends the engine between two of its ticks, under a lock, and the configuration engine is never removed; every engine but the configuration engine begins its log with `Started{Creator, CreatorTick, DirectiveID, Snapshot}`, the engine as it was made written as a checkpoint, which depends on the tick of its creator, so a recovery cuts the engine when that tick is lost; a replay makes an engine from that record or from its checkpoint, and the handover makes it again and tells the cluster which request made which engine (see [engines](../../docs/engine_lifecycle.md)) | `done` |
| No builder of a cluster: the code that runs it makes the cluster (`engine.NewCluster`, `NewReplayCluster`) and adds the world (`Cluster.CreateEngine`); the world makes the zones and the zonelayers by commands (`WorldZoneAddCommand`, `WorldZoneLayerAddCommand`) and a zonelayer tells its zone that it is there when it starts; `cmd/world` starts a new cluster (`-new-dir`) or from a replay (`-replay-dir`) | `done` |
| An exit is data that the world adds while the game runs (`WorldZoneExitAddCommand`, `ZoneExitAddAction`, `zone.State.AddExit`, `event.ExitAdded`), not part of the map; the map is written with its zone and has no exit; a tile is `tilemap.TileSize` units, which is never changed | `done` |
| A state holds no ID: the ID is the engine's (`Engine.New(state, id, env)`, `Context.Self`, the header of a checkpoint), `State.ID()` is gone, an `EntityID` is the number that its state counted, an event holds no owner (the emitter is told which engine made it), and the map of a zone is written with its state (`tilemap.Encode`), so a decoder takes no function | `done` |

## To build

What is left of the recovery of a cluster: a cluster is recovered and goes on live from the recovery line; what is not there yet is the measure below.

| Piece | What it is |
|---|---|
| To measure | the size of the unsettled messages in a checkpoint |

## Later

Status: `agreed`.

| Item | What it is |
|---|---|
| The rule that decides when the game makes a zone, a zonelayer or an exit | the mechanism is built, and only a command makes one (`WorldZoneAddCommand`, `WorldZoneLayerAddCommand`, `WorldZoneExitAddCommand`); the rule of the game that says when, and who asks, is not designed |
| Telling a player that a message could not be delivered | the kind `onfail.Notify`, `Bouncer.Tell` and `Noticer` were removed; to build again, planned: `Compensate` with an action that tells the actor, which the state turns into an event (R18); needs a concrete plan |
| Encoding and hash made asynchronous | `Append` keeps the struct, and a goroutine of the log encodes it and works out the hash from the copy of the state that the engine publishes and a copy it takes of what only it knows, in the order of the ticks; needs a rule that a struct given to `Append` is never changed (with a test), a bound on the list, and `engineHash` as a function of copies; measure the hash with a large zonelayer first |
| A version for each encoder | `Encode()` returns `(version, bytes)`, the version is written beside the bytes, and each `Decode` is given the version written for its own bytes; no one version for all the data; R32 changes with it |
| What a full ring lets go | today a ring (`shared/ring`: the chat of a zone and of a zonelayer, the private messages of the world) drops its oldest row when a new one does not fit, and a checkpoint writes every row it holds; to do: `Add` returns the row it lets go, and the state hands that row to a central store (the store of phase 6) instead of losing it; then what is past the ring lives in the store and not in the checkpoint |
| Encoder writers reused from a pool | `binary.Writer` is made as a literal at every place that encodes and its bytes are left to the collector; to do: `binary.NewWriter()` takes a writer from a `sync.Pool` with empty bytes, and `Recycle()` empties the bytes (keeping the capacity) and puts the writer back; every place that encodes calls `NewWriter` and then `defer w.Recycle()` at once; the bytes `B` are not exposed: `Len()` and `AppendTo(dst []byte) []byte` (an append, like `append`) are the only way out, so no slice of the pool's buffer can outlive `Recycle`, and what must live longer (what `Encode` returns, a frame handed to a log or a checkpoint) is appended to a buffer of the caller's; every use of `Writer{}` and of `.B` in code and tests is changed with it (pre-alpha, no shim); measure the allocations before and after |
