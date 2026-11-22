# Replay runner

Package `engine/replay/runner`. Runs the engines of a cluster again from tick 0 on the records of a replay log, and says whether each engine did what its log says. What is recorded: [replay_log](replay_log.md). The design: `plan/en/proposed-designs.md`.

## What it does

| Part | Value |
|---|---|
| entry | `runner.Run(builder, logs, options)` |
| the builder | describes the cluster that was recorded: the same world, zones, maps, zonelayers and seed; what it sets for the replay log, the emitters and the configuration is not used |
| the logs | `runner.Logs`: the records of each engine, in order; `runner.ReadLogs(dir)` reads the files of a `replay.FileLog`, and a memory log is grouped by engine |
| the result | `runner.Result`: the cluster (each engine at its last tick), the last tick of each engine, and the divergences |
| an error | the replay cannot run: a wrong builder, a record that cannot be given to its engine, a log of an engine that is not in the cluster, engines that wait for each other for ever |
| a divergence | the first difference of an engine between the run and its log; not an error |

- The engines are the real ones, with the same `Step`. The adapters on `Env` are what changes.
- The engines run one after another on the goroutine of the caller. An engine runs its next tick when the copies that tick read exist.
- An engine runs up to the last tick of its log, or of a read of it by another engine. A tick after the last record, with no record, is not run.

## Files

| File | Holds |
|---|---|
| `runner.go` | `Run`, `Options`, `Result` |
| `logs.go` | `Logs`, `ReadLogs` |
| `engine.go` | `Engine`, what the runner needs of an engine; an adapter for each scope that gives it a command through the `Send` of the engine; the list of the engines of a cluster |
| `run.go` | the run: indexing the logs, the order of the engines, the inputs of a tick |
| `router.go` | `replayRouter` and the key of a message |
| `stable.go` | `replayStable` and the key of a read |
| `config.go` | `replayConfig` |
| `verify.go` | `verifyLog` |
| `divergence.go` | the first difference of each engine |

## Tests

| Where | Holds |
|---|---|
| `runner_test.go` | the tests of the replay and its two benchmarks |
| `engine/enginetest` | what the tests of the engines and of the runner share: the world they run (`WorldBuilder`, `NewWorld`), the recorder of what the engines tell their clients, a replay log that writes in memory and to files (`NewTeeWorld`), and the scenario that runs a cluster through every kind of input (`RunScenario`) |

## What `engine` gives it

| Seam | Used for |
|---|---|
| `engine.MessageRouter`, `engine.StableRegistry`, `engine.ConfigSource` | the adapters on `Env` |
| `engine.CopySource` and `engine.NewStableStateView` | a registry that finds the copy a read names |
| `ClusterBuilder.BuildIn(env)`, `WorldID()`, `QueueSize()` | making the engines in the environment of the replay |
| `Step`, `Address` and `Send` of each engine, and `Cluster.Zones()` and `Cluster.ZoneLayers()` | running each engine and giving it its commands; the runner defines the interface it needs of them (`runner.Engine`) |

## Adapters

| Seam | Type in `Env` | In a replay |
|---|---|---|
| router | `MessageRouter` | `replayRouter`: holds the real `Router` to carry what the replay puts in the queue of an engine |
| published states | `StableRegistry` | `replayStable`: keeps the copies that some read asks for, by tick, and gives each read the copy its record names |
| configuration | `ConfigSource` | `replayConfig`: the snapshot of the version in effect for the engine about to run |
| replay log | `replay.Log` | `verifyLog`: compares each record the engine makes with the next one of its log |
| emitters | `WorldEmitter`, `ZoneEmitter`, `ZoneLayerEmitter` | `NopEmitter` |

## Inputs of a tick

Before an engine runs a tick, the records of that tick are given to it, in the order they are in the log:

| Record | What the runner does |
|---|---|
| `KindConfig` | the engine reads the version from now on; `runner.Options.ConfigVersions` says what it was made of (version 0 is the defaults) |
| `KindCommand` | `Send` of the engine |
| `KindInbound` | the message is put in the queue of the engine; the queue is as large as the busiest tick needs |
| `KindReturned` | a message sent in an earlier tick is given back at this one (`Router.Return`); one that is sent in this tick is refused at once, with the reason, when its sender sends it |
| `KindStableRead` | the read at this tick gets the copy of the tick in the record; the engine does not run before the source engine has run to that tick |

## What it checks

- Every record that the engine makes goes through `verifyLog`: the same kind, tick and content, in the same order. A hash is worked out and compared. A record in the log that the engine does not make, and one the engine makes that the log does not have, are divergences.
- A message that an engine sends is compared with the one that its receiver recorded as taken, by the bytes it is written as. A difference means the sender is not what it was.
- Only the first divergence of each engine is reported.

## Limits

- The configuration of a version must be given in `runner.Options`: the log holds the number of a version and not what it was made of. A version that is not given is a divergence.
- A change made on a state from outside its engine is in no log and no replay can make it.
- Ticks with no record are not run, so a state is compared at the ticks that record something; with `replay_log.hash_every_ticks` at 1 every tick records a hash.
- One goroutine: replay costs about what the cluster costs live, on one core.

Benchmarks: [replay_runner](benchmark/replay_runner.md).
