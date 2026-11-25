# Recovery

`platform/recovery`. Brings a cluster back from its directory: the replay logs and the checkpoints of its engines. It works out the recovery line, cuts the logs and the checkpoints to it, and runs each engine again from its latest checkpoint to the end of its log. What is recorded: [replay_log](replay_log.md). What a checkpoint holds: [checkpoint](checkpoint.md). How the engines are run again: [replay_runner](replay_runner.md).

## Entry points

| Call | What it does |
|---|---|
| `recovery.Recover(dir, builder, options)` | cuts to the line, chooses the start of each engine, and runs each engine from its start to its line with `runner.Run`; no log may be open for writing |
| `recovery.RecoveryLine(dir, codecs)` | the last tick of each log under `dir` that a recovery keeps |
| `recovery.CutToRecoveryLine(dir, codecs)` | cuts each log and each checkpoint to the line, writes the cache again, and returns the ticks |
| `recovery.CheckpointFiles(dir, starts)` | the `engine.CheckpointSource` of the checkpoint files: for each engine with a start above 0, the file of its checkpoint at that tick |
| `recovery.ReadLineCache(dir)`, `recovery.WriteLineCache(dir, cache)` | the cache of the line (below) |
| `recovery.NewLineCacheWriter(dir, cfg)` | the `replay.SafeObserver` that writes the cache |

| Field of `recovery.Result` | Holds |
|---|---|
| `Limits` | the last tick of each engine that was kept: the recovery line |
| `Starts` | the tick each engine was started at: its checkpoint, or 0 |
| `Replay` | the `runner.Result`: the cluster at the limits, the last tick of each engine, the divergences |

- `Options.ConfigVersions` says what each version of the configuration was made of (see [replay_runner](replay_runner.md#limits)).
- An error is a file that cannot be read or cut, a checkpoint that cannot make its engine (or is of another tick than its file says), or a replay that cannot run. An engine that did not do what its log says is a divergence in `Result.Replay`, not an error.

## Recovery line

- An engine starts at the last tick of its log and is cut before the first tick that depends (`Record.Dependency()`) on a tick of another engine that is cut away, or of an engine that has no log (its limit is 0). This is repeated until nothing changes, since an engine that was cut may be what another depends on. A tick is cut whole, with the commands of the clients that it took, and with everything after it.
- What is left is a state of the cluster in which no engine took a message that its sender did not send, or read a copy that its source did not publish. Each engine can be made again from its own log, with no other.
- It is the same rule as `FileLog.Safe`, from the other side: `Safe` is what the line is while the cluster runs, and the line is what `Safe` was when it stopped, or more.

## Cache of the line

| Key | Value |
|---|---|
| what it holds | the `Safe` of each engine at some time (`LineCache.Safe`); a tick that is safe is never cut, so the line of an engine is at or above it |
| file | `<dir>/recovery-line`, written whole or not at all ([atomicfile](atomicfile.md)) |
| what it saves | `RecoveryLine` reads the log of an engine from the tick after the cached one, and not from tick 1 |
| written by | `LineCacheWriter`, the `FileLogOptions.OnSafe` of the cluster's `replay.FileLog`: when some engine is `recovery.line_cache_every_ticks` (600) past what was written, and when the log is closed; and by `CutToRecoveryLine`, with the line |
| used when | every engine it names has a log and none of its ticks is above the last tick of that log; an engine it does not name is read from tick 1 |
| not used when | it is not readable, names an engine with no log, or says more than a log holds: it is logged as a warning and the logs are read whole |

## Cut

| What | Where it goes |
|---|---|
| the records of a log above its line | `<dir>/.archive/cut-<n>/<engine>/seg-*.<now>.pbsl.discarded-<offset>` ([segmentlog](segmentlog.md#cut)) |
| the checkpoints of an engine above its line | `<dir>/.archive/cut-<n>/<engine>/ckpt-<tick>.<now>.snap` |

- `<n>` counts from 1, one above the largest that is there, so what two cuts keep never meets. The directory is made only when something is cut. Nothing is deleted.
- A file put in the archive has the time of the move (UTC, to the nanosecond) before its extension, `<origin>.<now>.<ext>` (`atomicfile.Stamped`), so that what is put aside twice under one name is not written over.
- `<dir>/.archive` is not read as the log of an engine or as a checkpoint (`replay.Engines`, `checkpoint.Ticks`).
- A checkpoint above the line holds what was cut. A cut is made from the end, so that a crash in the middle leaves a cluster that is whole and longer than it should be, and the cut is made again.
- A log already at its line and an engine with no checkpoint above it are not touched; a second cut of the same files changes nothing.

## Start of an engine

| Rule | |
|---|---|
| first choice | the latest checkpoint of the engine at or below its limit, or tick 0 when it has none |
| an engine that reads a copy before another's start | a record `StableRead{Source, CopyTick}` of a tick after the reader's start needs the copy of `Source` at `CopyTick`: `Source` must start at or before it, so it starts at its latest checkpoint at or below `CopyTick` (or 0), and its own reads after that are looked at in turn |
| ends | a start only goes down, so the repeat ends |
| a copy at the start itself | the engine made from a checkpoint publishes its state before its first tick (`PublishState`) |

## Not here yet

- Going live after the replay: the engines made again in the real environment, the logs written on after the cut, the unsettled messages sent again and the acks made again ([roadmap](../plan/en/roadmap.md)).

Benchmarks: [recovery](benchmark/recovery.md).
