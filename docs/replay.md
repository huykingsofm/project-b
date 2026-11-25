# Replay

Package `platform/replay`. Brings a cluster back from its directory: the event logs and the checkpoints of its engines. It works out the recovery line (the plan), cuts the logs and the checkpoints to it, and runs each engine again from its latest checkpoint to its limit on its own log, saying whether each engine did what its log says. What is recorded: [eventlog](eventlog.md). What a checkpoint holds: [checkpoint](checkpoint.md). The design: `plan/en/proposed-designs.md`.

A replay always runs on a plan, and a plan is made only by `MakePlan` or `PlanFrom`, which work out the recovery line of the logs: a replay never runs on logs in which an engine took what its sender did not send, or read a copy that its source did not publish.

## Entry points

| Call | What it does |
|---|---|
| `replay.MakePlan(dir, codecs)` | the plan of the files under `dir`: the limits (the recovery line) and the start of each engine; reads, writes nothing; no log may be open for writing |
| `replay.PlanFrom(logs, starts, checkpoints)` | the plan of logs that are not files (memory, or as a test makes them), with the starts and the checkpoints that are given; it cannot be cut |
| `(*Plan).Limits()`, `(*Plan).Starts()` | the last tick of each engine that is kept, and the tick each engine starts at (its checkpoint, or 0) |
| `(*Plan).Cut()` | brings the files to the plan: each log is cut to its limit, each checkpoint above its limit goes to the archive, the cache of the line is written again |
| `replay.Run(builder, plan, options)` | runs the engines again on the plan and returns the `Result`; changes nothing on the disk |
| `replay.Recover(dir, builder, options)` | `MakePlan`, `Cut` and `Run`; an engine that did not do what its log says is a `*DivergenceError`, returned with the result |
| `replay.FileLogs(dir, codecs)`, `replay.MemoryLogs` | where the records come from (below) |
| `replay.ReadLineCache(dir)`, `replay.WriteLineCache(dir, cache)`, `replay.NewLineCacheWriter(dir, cfg)` | the cache of the line (below) |

| Part | Value |
|---|---|
| the builder | a `replay.Builder` (`Seed`, `QueueSize`, `Codecs`, `BuildIn(env, checkpoints)`), which the game gives (`game.ClusterBuilder`): it describes the cluster that was recorded; what it sets for the event log, the emitter and the configuration is not used |
| the logs | `replay.Logs`: for each engine, its last tick (`Last`) and an `eventlog.Source` that gives its records in order, one at a time, from a tick (`Open(engine, from)`: the tick `from` and the ones after it); `FileLogs` reads the files of an `eventlog.FileLog`, and `MemoryLogs` holds records in memory |
| `Options` | `ConfigVersions`: what each version of the configuration was made of (version 0 is the defaults) |
| `Result` | the plan, the cluster (each engine at its limit), the last tick of each engine, and the divergences |
| an error | the replay cannot run: a wrong builder, a record that cannot be given to its engine, a log of an engine that is not in the cluster, engines that wait for each other for ever, a file that cannot be read or cut, a checkpoint that cannot make its engine (or is of another tick than its file says) |
| a divergence | the first difference of an engine between the run and its log; not an error of `Run`, a `*DivergenceError` of `Recover` |

- The engines are the real ones, with the same `Step`. The adapters on `Env` are what changes.
- The engines run one after another on the goroutine of the caller. An engine runs its next tick when the copies that tick read exist.
- An engine runs from its start to its limit. A limit is a tick that the log has, so a tick with no record is not run past the last one.
- Each log is read a tick at a time and never whole: one pass indexes it, one runs it. What is kept follows the ticks that are loaded and the reads still to come, not the length of the log.

## Files

| File | Holds |
|---|---|
| `plan.go` | `Plan`, `MakePlan`, `PlanFrom`; the recovery line; the choice of the start of each engine; the checkpoint files as a source |
| `cut.go` | `Plan.Cut` and the archive of what it cuts |
| `line_cache.go` | the cache of the line and its writer |
| `replay.go` | `Run`, `Recover`, `Builder`, `Options`, `Result`, `DivergenceError` |
| `run.go` | the run: `engineRun` (what a replay holds of one engine: the engine, its limit, the tick it has run to, its log and the records of the tick loaded, the version of the configuration) in a `stablemap.Map` that keeps the order of `Step`; how an engine that starts from a checkpoint begins; the inputs of a tick |
| `index.go` | the pass over each log before the run: how many reads each copy has, which messages came back, a hash of each message as it was taken, the size of the queues, the seed |
| `logs.go` | `Logs`, `MemoryLogs`, `FileLogs`, and the `tickReader` that reads the records of one engine a tick at a time |
| `adapters.go` | `replayRouter`, `replayStable`, `replayConfig` and `verifyLog` |
| `divergence.go` | the first difference of each engine |

## Tests

| Where | Holds |
|---|---|
| `platform/replay/recovery_test.go`, `toy_test.go` | the line, the cut, the cache and the start rule, on toy engines |
| `platform/replay/stable_test.go`, `logs_test.go` | the store of a replay, and the memory logs |
| `server/game/replay_runner_test.go`, `recovery_test.go` | the replay and the recovery of the world of this game, and their benchmarks |
| `server/game/testkit` | what the tests of the game share: the world they run (`WorldBuilder`, `NewWorld`), an event log that writes in memory and to files (`NewTeeWorld`), and the scenario that runs a cluster through every kind of input (`RunScenario`) |

## What the platform gives it

| Seam | Used for |
|---|---|
| `engine.MessageRouter`, `engine.StableStore`, `engine.ConfigSource` | the adapters on `Env` |
| `engine.CopySource` and `engine.NewStableView` | a store that finds the copy a read names |
| `Builder.BuildIn(env, checkpoints)` and `engine.CheckpointSource` | making the engines in the environment of the replay, each from its checkpoint or new |
| `engine.Stepper` (`Step`, `StateID`, `TypeName`, `SendBaseCommand`) and `Cluster.Engines()` | running each engine and giving it its commands; a command of another type of state is an error of the replay, not a panic |

## Adapters

| Seam | Type in `Env` | In a replay |
|---|---|---|
| router | `MessageRouter` | `replayRouter`: holds the real `Router` to carry what the replay puts in the queue of an engine |
| published states | `StableStore` | `replayStable`: keeps the copies that some read asks for, by tick, and gives each read the copy its record names |
| configuration | `ConfigSource` | `replayConfig`: the snapshot of the version in effect for the engine about to run |
| event log | `eventlog.Log` | `verifyLog`: compares each record the engine makes with the next one of its log |
| emitter | `Emitter` | `NopEmitter` |

## Inputs of a tick

Before an engine runs a tick, the records of that tick are given to it, in the order they are in the log:

| Record | What the replay does |
|---|---|
| `KindConfig` | the engine reads the version from now on; `Options.ConfigVersions` says what it was made of |
| `KindCommand` | `SendBaseCommand` of the engine |
| `KindInbound` | the message is put in the queue of the engine; the queue is as large as the busiest tick needs |
| `KindReturned` | a message sent in an earlier tick is given back at this one (`Router.Return`); one that is sent in this tick is refused at once, with the reason, when its sender sends it |
| `KindStableRead` | the read at this tick gets the copy of the tick in the record; the engine does not run before the source engine has run to that tick |

## An engine that starts from a checkpoint

Before the first tick, for each engine that starts above tick 0:

| Step | What the replay does |
|---|---|
| tick | the engine made by the builder must be at the start: another tick is an error of the replay, not a divergence |
| records up to the start | not given to the engine, nor indexed: the checkpoint holds what they did |
| hash | the log has the hash of the engine at the tick of its checkpoint (an engine records it at every tick it takes one); the engine that was made must have the same: a different one, or none in the log, is a divergence |
| configuration | the version the engine had read (`ConfigVersionSeen`) is the one in effect, so a version it read before the start is not recorded again |
| messages that went | the messages the checkpoint holds as sent and not acked that the log says came back are kept, to be given back at the tick the log says |
| copy | the state is published (`PublishState`), for the reads of other engines at the start tick |

## What it checks

- Every record that the engine makes goes through `verifyLog`: the same kind, tick and content, in the same order. A hash is worked out and compared. A record in the log that the engine does not make, and one the engine makes that the log does not have, are divergences.
- A message that an engine sends is compared with the one that its receiver recorded as taken (`inboundMessageHash`), by a hash of the bytes it is written as. A difference means the sender is not what it was.
- The files of a log hold the seed of the run (`eventlog.Seeded`); a builder with another seed is refused. A memory log has none, and the seed of its builder is trusted.
- Only the first divergence of each engine is reported.

## Recovery line

- An engine starts at the last tick of its log and is cut before the first tick that depends (`Record.Dependency()`) on a tick of another engine that is cut away, or of an engine that has no log (its limit is 0). This is repeated until nothing changes, since an engine that was cut may be what another depends on. A tick is cut whole, with the commands of the clients that it took, and with everything after it.
- What is left is a state of the cluster in which no engine took a message that its sender did not send, or read a copy that its source did not publish. Each engine can be made again from its own log, with no other.
- It is the same rule as `FileLog.Safe`, from the other side: `Safe` is what the line is while the cluster runs, and the line is what `Safe` was when it stopped, or more.

## Cache of the line

| Key | Value |
|---|---|
| what it holds | the `Safe` of each engine at some time (`LineCache.Safe`); a tick that is safe is never cut, so the line of an engine is at or above it |
| file | `<dir>/recovery-line`, written whole or not at all ([atomicfile](atomicfile.md)) |
| what it saves | `MakePlan` reads the log of an engine from the tick after the cached one, and not from tick 1, to work out the line |
| written by | `LineCacheWriter`, the `FileLogOptions.OnSafe` of the cluster's `eventlog.FileLog`: when some engine is `replay.line_cache_every_ticks` (600) past what was written, and when the log is closed; and by `Plan.Cut`, with the limits |
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

## Limits

- The configuration of a version must be given in `Options`: the log holds the number of a version and not what it was made of. A version that is not given is a divergence.
- A change made on a state from outside its engine is in no log and no replay can make it.
- Ticks with no record are not run, so a state is compared at the ticks that record something; with `eventlog.hash_every_ticks` at 1 every tick records a hash.
- One goroutine: replay costs about what the cluster costs live, on one core.
- Going live after the replay (the engines made again in the real environment, the logs written on after the cut, the unsettled messages sent again and the acks made again) is not here yet ([roadmap](../plan/en/roadmap.md)).

Benchmarks: [replay](benchmark/replay.md).
