# Replay runner

Package `platform/replay/runner`. Runs the engines of a cluster again from tick 0 on the records of a replay log, and says whether each engine did what its log says. What is recorded: [replay_log](replay_log.md). The design: `plan/en/proposed-designs.md`.

## What it does

| Part | Value |
|---|---|
| entry | `runner.Run(builder, logs, options)` |
| the builder | a `runner.Builder` (`Seed`, `QueueSize`, `Codecs`, `BuildIn(env)`), which the game gives (`game.ClusterBuilder`): it describes the cluster that was recorded; what it sets for the replay log, the emitter and the configuration is not used |
| the logs | `runner.Logs`: for each engine, a `runner.Source` that gives its records in order, one at a time; `runner.FileLogs(dir, codecs)` reads the files of a `replay.FileLog`, and `runner.MemoryLogs` holds records in memory |
| the result | `runner.Result`: the cluster (each engine at its last tick), the last tick of each engine, and the divergences |
| an error | the replay cannot run: a wrong builder, a record that cannot be given to its engine, a log of an engine that is not in the cluster, engines that wait for each other for ever |
| a divergence | the first difference of an engine between the run and its log; not an error |

- The engines are the real ones, with the same `Step`. The adapters on `Env` are what changes.
- The engines run one after another on the goroutine of the caller. An engine runs its next tick when the copies that tick read exist.
- An engine runs up to the last tick of its log, or of a read of it by another engine. A tick after the last record, with no record, is not run.
- Each log is read twice, a tick at a time and never whole: one pass indexes it, one runs it. What is kept follows the ticks that are loaded and the reads still to come, not the length of the log.

## Files

| File | Holds |
|---|---|
| `runner.go` | `Run`, `Options`, `Result` |
| `logs.go` | `Source`, `Logs`, `Seeded`, `MemoryLogs`, `FileLogs` |
| `index.go` | the pass over each log before the run: the last tick of each engine, how many reads each copy has, which messages came back, a hash of each message as it was taken, the size of the queues, the seed |
| `cursor.go` | the records of one engine, a tick at a time |
| `run.go` | the run: the order of the engines, the records of a tick, the inputs of a tick |
| `router.go` | `replayRouter` and the key of a message |
| `stable.go` | `replayStable`, which lets a copy go as soon as its last read is made, and the key of a read |
| `config.go` | `replayConfig` |
| `verify.go` | `verifyLog` and the window of the tick that is run |
| `divergence.go` | the first difference of each engine |

## Tests

| Where | Holds |
|---|---|
| `platform/replay/runner/stable_test.go` | the store of a replay, on a state of its own |
| `server/game/replay_runner_test.go` | the tests of the replay of the world of this game and its two benchmarks |
| `server/game/testkit` | what the tests of the game share: the world they run (`WorldBuilder`, `NewWorld`), the recorder of what the engines tell their clients, a replay log that writes in memory and to files (`NewTeeWorld`), and the scenario that runs a cluster through every kind of input (`RunScenario`) |

## What the platform gives it

| Seam | Used for |
|---|---|
| `engine.MessageRouter`, `engine.StableStore`, `engine.ConfigSource` | the adapters on `Env` |
| `engine.CopySource` and `engine.NewStableView` | a store that finds the copy a read names |
| `runner.Builder.BuildIn(env)` | making the engines in the environment of the replay |
| `engine.Stepper` (`Step`, `StateID`, `TypeName`, `SendBaseCommand`) and `Cluster.Engines()` | running each engine and giving it its commands; a command of another type of state is an error of the replay, not a panic |

## Adapters

| Seam | Type in `Env` | In a replay |
|---|---|---|
| router | `MessageRouter` | `replayRouter`: holds the real `Router` to carry what the replay puts in the queue of an engine |
| published states | `StableStore` | `replayStable`: keeps the copies that some read asks for, by tick, and gives each read the copy its record names |
| configuration | `ConfigSource` | `replayConfig`: the snapshot of the version in effect for the engine about to run |
| replay log | `replay.Log` | `verifyLog`: compares each record the engine makes with the next one of its log |
| emitter | `Emitter` | `NopEmitter` |

## Inputs of a tick

Before an engine runs a tick, the records of that tick are given to it, in the order they are in the log:

| Record | What the runner does |
|---|---|
| `KindConfig` | the engine reads the version from now on; `runner.Options.ConfigVersions` says what it was made of (version 0 is the defaults) |
| `KindCommand` | `SendBaseCommand` of the engine |
| `KindInbound` | the message is put in the queue of the engine; the queue is as large as the busiest tick needs |
| `KindReturned` | a message sent in an earlier tick is given back at this one (`Router.Return`); one that is sent in this tick is refused at once, with the reason, when its sender sends it |
| `KindStableRead` | the read at this tick gets the copy of the tick in the record; the engine does not run before the source engine has run to that tick |

## What it checks

- Every record that the engine makes goes through `verifyLog`: the same kind, tick and content, in the same order. A hash is worked out and compared. A record in the log that the engine does not make, and one the engine makes that the log does not have, are divergences.
- A message that an engine sends is compared with the one that its receiver recorded as taken, by a hash of the bytes it is written as. A difference means the sender is not what it was.
- The files of a log hold the seed of the run (`runner.Seeded`); a builder with another seed is refused. A memory log has none, and the seed of its builder is trusted.
- Only the first divergence of each engine is reported.

## Limits

- The configuration of a version must be given in `runner.Options`: the log holds the number of a version and not what it was made of. A version that is not given is a divergence.
- A change made on a state from outside its engine is in no log and no replay can make it.
- Ticks with no record are not run, so a state is compared at the ticks that record something; with `replay_log.hash_every_ticks` at 1 every tick records a hash.
- One goroutine: replay costs about what the cluster costs live, on one core.

Benchmarks: [replay_runner](benchmark/replay_runner.md).
