# Benchmarks of the replay

Conventions and how to run: [index](index.md). Behavior: [replay](../replay.md).

## Replay of a whole cluster

| Name | Package | Measures |
|---|---|---|
| `BenchmarkRunScenarioLive` | `server/game` | building the world of the tests and running the scenario of `runScenario` (chat in each scope, a shield, an attack, a pickup, a run of movement, a new document of the config, a character that crosses to another zone, 90 ticks of each of 6 engines), with a memory event log |
| `BenchmarkReplayCluster` | `server/game` | building the same world and replaying the records of that scenario with `replay.Run` |

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| RunScenarioLive | 742,848 | 722,083 | 9,355 |
| ReplayCluster | 726,059 | 691,060 | 8,741 |

- A replay runs 477 engine ticks, so about 1.5 µs for each, the building of the cluster included.
- A replay costs about what the run does (2 percent less). It reads each log twice, once to index it and once to run it, works out every hash it compares, and hashes each message that it keeps for comparing; and it does not run the scenario's own sends. The plan (`PlanFrom`) reads each log once more, to work out the line, which this benchmark does not include.
- What it keeps no longer grows with the log: a copy of an engine goes as soon as its last read is made, and the logs are read a tick at a time. A scenario this short cannot show it; `TestACopyIsLetGoOfAsSoonAsItsLastReadIsMade` shows that no copy is kept when the run is over.
- What still grows with the log is the index: about 40 bytes for each message that was taken, a count for each copy that was read, and the keys of the messages that came back. For a log of 10 million messages that is about 400 MB; keeping the index on disk, or only for the messages that a sender and a receiver of the same tick range share, would take it off, when a log that long has to be replayed whole.

## Recovery

All of them are in `server/game` and run on the files of a world that ran the scenario of `testkit.RunScenario` and then 3,000 ticks more (3,100 ticks of each of 6 engines), with a checkpoint every 500 ticks, written to disk by an `eventlog.FileLog` and a `checkpoint.FileWriter`.

| Name | Measures |
|---|---|
| `BenchmarkRecoveryLineWithTheCacheAtTheEndOfTheLogs` | `replay.MakePlan` (the line and the start of each engine) with the cache that the log wrote when it was closed: no log has a tick after what it says is safe, the best case |
| `BenchmarkRecoveryLineWithNoCache` | `replay.MakePlan` with no cache: every log is read from tick 1, the worst case |
| `BenchmarkReplayFromTickZero` | `replay.Run` on the files, each engine from tick 0 |
| `BenchmarkReplayFromTheCheckpoints` | `replay.Run` on the files, each engine from the start that `replay.MakePlan` chose |
| `BenchmarkRecover` | `replay.Recover`: the plan with its cache, the cut (nothing to cut), and the replay from the starts |

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| RecoveryLineWithTheCacheAtTheEndOfTheLogs | 3,646,170 | 9,508,069 | 16,797 |
| RecoveryLineWithNoCache | 9,518,047 | 23,881,065 | 112,193 |
| ReplayFromTickZero | 38,349,624 | 56,270,583 | 520,637 |
| ReplayFromTheCheckpoints | 5,173,183 | 9,411,369 | 49,942 |
| Recover | 12,825,743 | 19,401,192 | 69,597 |

The line and the choice of the start of each engine are one call now (`MakePlan`), so the first two rows hold the starts too: they read the logs after each checkpoint to find the reads of copies, which the line alone did not (it cost 1,901,000 and 7,062,000 ns/op before).

- A replay from the checkpoints costs 13 percent of one from tick 0: each engine runs the ticks after its checkpoint, at most 500, instead of 3,100, and the index reads the logs from there too.
- The cache takes the plan from 9.5 ms to 3.6 ms, 38 percent. What is left of it is opening the 6 logs, finding their last tick, and reading the logs after each checkpoint for the choice of the starts, which a cache of the line cannot take off. A cache that is up to a period behind costs between the two.
- `Recover` costs about a third of the replay from tick 0, with the plan, the cache, the cut and the files of the checkpoints included. The part that grows with the length of the log is the replay; what the line costs follows the ticks after the cache, and what the starts cost follows the ticks after each checkpoint.
