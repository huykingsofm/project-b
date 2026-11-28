# Replay

Package `platform/journal/replay`. Brings a cluster back from its directory: the event logs and the checkpoints of its engines. It works out the recovery line (the plan), cuts the logs and the checkpoints to it, and runs each engine again from its latest checkpoint to its limit on its own log, saying whether each engine did what its log says. What is recorded: [eventlog](eventlog.md). What a checkpoint holds: [checkpoint](checkpoint.md). The design: `plan/en/proposed-designs.md`.

A replay always runs on a plan, and a plan is made only by `MakeRecoveryPlan` or `RecoveryPlanFrom`, which work out the recovery line of the logs: a replay never runs on logs in which an engine took what its sender did not send, or read a copy that its source did not publish.

## Entry points

| Call | What it does |
|---|---|
| `replay.MakeRecoveryPlan(dir, codecs)` | the plan of the files under `dir`: the limits (the recovery line) and the start of each engine; reads, writes nothing; no log may be open for writing |
| `replay.RecoveryPlanFrom(logs, starts, checkpoints)` | the plan of logs that are not files (memory, or as a test makes them), with the starts and the checkpoints that are given; it cannot be cut |
| `(*RecoveryPlan).Limits()`, `(*RecoveryPlan).Starts()` | the last tick of each engine that is kept, and the tick each engine starts at (its checkpoint, or 0) |
| `(*RecoveryPlan).Cut()` | brings the files to the plan: each log is cut to its limit, each checkpoint above its limit goes to the archive, the cache of the line is written again |
| `replay.Run(options, plan)` | runs the engines again on the plan and returns the `Result`; changes nothing on the disk |
| `replay.Recover(dir, options)` | `MakeRecoveryPlan`, `Cut` and `Run`; an engine that did not do what its log says is a `*DivergenceError`, returned with the result |
| `(*Result).Handover(env, store)` | makes the engines again in `env`, the environment of a cluster that runs (`engine.NewEnvironment`), with an orchestrator for the directory of the plan and `store`, and returns the cluster, which has not run yet (below) |
| `eventlog.NewFileLogs(dir, codecs)`, `eventlog.MemoryLogs`, `eventlog.MemLog` | where the records come from: the files of a `FileLog`, and records in memory (below) |
| `replay.ReadLineCache(dir)`, `replay.WriteLineCache(dir, cache)`, `replay.NewLineCacheWriter(dir, store)` | the cache of the line (below); `store` is the `config.Store` that the cluster is made with, which gives the period; nil means the default |

| Part | Value |
|---|---|
| the options | an `engine.Options` (`Seed`, `QueueSize`, `Codecs`, `Engines`): what the cluster that was recorded was made with; what it sets for the event log, the emitter and the checkpoints is not used. No builder: each engine is made from its checkpoint, or from the record that begins its log, through the registry (`engine.EngineRegistry`) |
| the logs | `replay.Logs`: for each engine, its last tick (`Last`) and an `eventlog.Source` that gives its records in order, one at a time, from a tick (`Open(engine, from)`: the tick `from` and the ones after it); `eventlog.FileLogs` reads the files of an `eventlog.FileLog`, and `eventlog.MemoryLogs` and `eventlog.MemLog` hold records in memory |
| `Result` | the plan, the cluster (each engine at its limit), the last tick of each engine, the engines it removed (`Retired`), and the divergences |
| an error | the replay cannot run: wrong options, a record that cannot be given to its engine, a log that begins with no record of how its engine was made and has no checkpoint to start from, engines that wait for each other for ever, a file that cannot be read or cut, a checkpoint that cannot make its engine (or is of another tick than its file says) |
| a divergence | the first difference of an engine between the run and its log; not an error of `Run`, a `*DivergenceError` of `Recover` |

- The engines are the real ones, with the same `Step`. The adapters on `Environment` are what changes.
- The engines run one after another on the goroutine of the caller. An engine runs its next tick when the copies that tick read exist.
- An engine runs from its start to its limit. A limit is a tick that the log has, so a tick with no record is not run past the last one.
- Each log is read a tick at a time and never whole: one pass indexes it, one runs it. What is kept follows the ticks that are loaded and the reads still to come, not the length of the log.

## What the platform gives it

| Seam | Used for |
|---|---|
| `router.MessageRouter`, `stable.Store` | the adapters on `Environment` |
| `stable.CopySource` and `stable.NewView` | a store that finds the copy a read names |
| `cluster.NewReplayCluster`, `Cluster.AddEngineByCheckpoint` and `engine.EngineRegistry` | making the engines in the environment of the replay, each from its checkpoint or from the record that begins its log |
| `Cluster.Engines()`, whose handles (`engine.EngineHandle`, which holds only what the cluster needs) the replay converts to its own interfaces (`runEngineHandle`: `Step`, `StateID`, `TypeName`, `Tick`, `Hash`, `GetSentMessages`, `PublishState`, `SendBaseCommand`, `RequestRetirement`, `RetireIfRequested`; `handoverEngineHandle`: `StateID`, `Tick`, `EncodeCheckpoint`, `PublishState`, `WaitsForDirectiveResponse`, `Adopt`) | running each engine and giving it its commands; a command of another type of state is an error of the replay, not a panic |

## Adapters

| Seam | Type in `Environment` | In a replay |
|---|---|---|
| router | `MessageRouter` | `replayRouter`: holds the real `Router` to carry what the replay puts in the queue of an engine |
| published states | `stable.Store` | `replayStable`: keeps the copies that some read asks for, by tick, and gives each read the copy its record names |
| event log | `eventlog.Log` | `verifyLog`: compares each record the engine makes with the next one of its log |
| emitter | `Emitter` | `NopEmitter` |

## Inputs of a tick

Before an engine runs a tick, the records of that tick are given to it, in the order they are in the log:

| Record | What the replay does |
|---|---|
| `KindCommand` | `SendBaseCommand` of the engine |
| `KindInbound` | the message is put in the queue of the engine; the queue is as large as the busiest tick needs |
| `KindReturned` | a message sent in an earlier tick is given back at this one (`Router.Return`); one that is sent in this tick is refused at once, with the reason, when its sender sends it |
| `KindInit` | the record of the tick 1 of the log of an engine: the replay makes the engine from the checkpoint inside it (the engine as it was made) when it starts from no checkpoint; the record, and `KindStarted`, are left out of the records that the engine is compared with ([engines](engine_lifecycle.md)) |
| `KindDirective` | a response of the cluster to a request of the engine: put in the directive router of the replay, which the engine takes it from at its tick; the requests and the acknowledgements of the engines are thrown away, since a replay has no cluster |
| `KindRetired` | the engine is removed (`Cluster.Remove`) and runs no tick: the frame of the tick holds this record and no other, and it is the last of the log (the index refuses a log in which it is not); the cluster of the result does not hold the engine, and its ID is in `Result.Retired` |
| `KindStableRead` | the read at this tick gets the copy of the tick in the record; the engine does not run before the source engine has run to that tick; the configuration is read so, from the copy of the coordinator engine ([coordinatorengine](coordinatorengine.md)) |

## An engine that starts from a checkpoint

Before the first tick, for each engine that starts above tick 0:

| Step | What the replay does |
|---|---|
| tick | the engine made from the checkpoint must be at the start: another tick is an error of the replay, not a divergence |
| records up to the start | not given to the engine, nor indexed: the checkpoint holds what they did |
| hash | the log has the hash of the engine at the tick of its checkpoint (an engine records it at every tick it takes one); the engine that was made must have the same: a different one, or none in the log, is a divergence |
| messages that went | the messages the checkpoint holds as sent and not acked that the log says came back are kept, to be given back at the tick the log says |
| copy | the state is published (`PublishState`), for the reads of other engines at the start tick |

## What it checks

- Every record that the engine makes goes through `verifyLog`: the same kind, tick and content, in the same order. A hash is worked out and compared. A record in the log that the engine does not make, and one the engine makes that the log does not have, are divergences.
- A message that an engine sends is compared with the one that its receiver recorded as taken (`inboundMessageHash`), by a hash of the bytes it is written as. A difference means the sender is not what it was.
- The files of a log hold the seed of the run (`eventlog.Seeded`); options with another seed are refused. A memory log has none, and the seed of the options is trusted.
- Only the first divergence of each engine is reported.

## Recovery line

- An engine starts at the last tick of its log and is cut before the first tick that depends (`Record.Dependency()`) on a tick of another engine that is cut away, or of an engine that has no log (its limit is 0). This is repeated until nothing changes, since an engine that was cut may be what another depends on. A tick is cut whole, with the commands of the clients that it took, and with everything after it.
- What is left is a state of the cluster in which no engine took a message that its sender did not send, or read a copy that its source did not publish. Each engine can be made again from its own log, with no other.
- It is the same rule as `FileLog.Safe`, from the other side: `Safe` is what the line is while the cluster runs, and the line is what `Safe` was when it stopped, or more.

## Cache of the line

| Key | Value |
|---|---|
| what it holds | the `Safe` of each engine at some time (`LineCache.Safe`); a tick that is safe is never cut, so the line of an engine is at or above it |
| file | `<dir>/recovery-line`, written whole or not at all ([atomicfile](shared.md#atomicfile)) |
| what it saves | `MakeRecoveryPlan` reads the log of an engine from the tick after the cached one, and not from tick 1, to work out the line |
| written by | `LineCacheWriter`, the `FileLogOptions.OnSafe` of the cluster's `eventlog.FileLog`: when some engine is `replay.line_cache_every_ticks` (600) past what was written, and when the log is closed; and by `RecoveryPlan.Cut`, with the limits |
| used when | every engine it names has a log and none of its ticks is above the last tick of that log; an engine it does not name is read from tick 1 |
| not used when | it is not readable, names an engine with no log, or says more than a log holds: it is logged as a warning and the logs are read whole |

## Cut

| What | Where it goes |
|---|---|
| the records of a log above its limit | `<dir>/.archive/cut-<n>/<engine>/seg-*.<now>.pbsl.discarded-<offset>` ([segmentlog](segmentlog.md#cut)) |
| the checkpoints of an engine above its limit | `<dir>/.archive/cut-<n>/<engine>/ckpt-<tick>.<now>.snap` |

- `<n>` counts from 1, one above the largest that is there, so what two cuts keep never meets. The directory is made only when something is cut. Nothing is deleted.
- A file put in the archive has the time of the move (UTC, to the nanosecond) before its extension, `<origin>.<now>.<ext>` (`atomicfile.Stamped`), so that what is put aside twice under one name is not written over.
- `<dir>/.archive` is not read as the log of an engine or as a checkpoint (`eventlog.Engines`, `checkpoint.Ticks`).
- A checkpoint above the line holds what was cut. A cut is made from the end, so that a crash in the middle leaves a cluster that is whole and longer than it should be, and the cut is made again.
- A log already at its limit and an engine with no checkpoint above it are not touched; a second cut of the same files changes nothing.

## Start of an engine

| Rule | |
|---|---|
| first choice | the latest checkpoint of the engine at or below its limit, or tick 0 when it has none |
| an engine that reads a copy before another's start | a record `StableRead{Source, CopyTick}` of a tick after the reader's start needs the copy of `Source` at `CopyTick`: `Source` must start at or before it, so it starts at its latest checkpoint at or below `CopyTick` (or 0), and its own reads after that are looked at in turn |
| ends | a start only goes down, so the repeat ends |
| a copy at the start itself | the engine made from a checkpoint publishes its state before its first tick (`PublishState`) |

## Handover

`Result.Handover(env, store)` goes live after a recovery. A replayed engine never runs live, so the engines are made again.

| Step | What it does |
|---|---|
| refused | a result with a divergence is not handed over |
| checkpoint | each engine of the replayed cluster is checkpointed at its limit (`Engine.EncodeCheckpoint`) |
| engines | `cluster.NewCluster(env, orchestrator, coordinatorCheckpoint)` makes the cluster and `Cluster.AddEngineByCheckpoint` each of the other engines from its checkpoint, with the router, the published states, the emitter, the acks and the checkpoint writer of `env` (`engine.NewEnvironment` makes such an environment) |
| sent messages | in the checkpoint of each replayed engine: the replay has an `AckBoard`, so an engine holds the messages it sent that no receiver took in the part of its log that was kept, as a live one does ([unsettled messages](unsettled_messages.md)) |
| copies | each engine that ran a tick publishes its state (`PublishState`), for the engines that read it; one at the tick 0 publishes nothing, so a read that records the tick 0 means that it found no copy |
| engines that never ran | an engine with `Init` and nothing after it is kept and run when the log of the engine that asked holds the response (also when that engine retired after), and dropped, with `Retired` written to its log, when the request still waits there or no response is in its log ([engines](engine_lifecycle.md)) |
| IDs | the orchestrator of the cluster goes on from its file (`state-id-marks`); the recovery drops an engine whose ID is above it |
| removed engines | the engines in `Result.Retired` are not made again: the router refuses a message for one as `DeliveryFailureRetired` and its log is not written on |
| after | the caller runs the cluster; each engine goes on from its own tick, and the event log of `env` writes on after the cut |

| Part | What happens |
|---|---|
| the first flush | each sent message is sent again, with its number ([unsettled messages](unsettled_messages.md#sending-again)) |
| a receiver that took it | `restoreFrom` raised its ack on the board, so the sender lets go of the message; a copy that still arrives is dropped by its number |
| a receiver that did not take it (its log was cut, or the message was lost) | takes it |
| a message that came back in a tick that was kept | let go of by its sender when it came back: not in the checkpoint |
| the queues | empty: what was not taken from them is not in any log |
| the configuration | the engine has what its checkpoint holds; a later document of the store is sent by the cluster as a command |

- A read of an engine that was removed names a copy of a tick that it ran, or none: what it published goes when it is removed, and it publishes nothing at the tick of its removal (the index refuses a read of a copy at that tick or after).
- A replayed engine sends its unacked messages again, as a live one does, into the router of the replay, which compares each with what its receiver recorded and carries nothing.
- The hash of an engine that is made again is the hash of the replayed one: the test of `server/game/handover_test.go` checks it, and the run does not.

## Limits

- A change made on a state from outside its engine is in no log and no replay can make it.
- Ticks with no record are not run, so a state is compared at the ticks that record something; with `eventlog.hash_every_ticks` at 1 every tick records a hash.
- One goroutine: replay costs about what the cluster costs live, on one core.

Benchmarks: [replay](benchmark/replay.md).
