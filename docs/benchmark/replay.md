# Benchmarks of the replay

Conventions and how to run: [index](index.md). Behavior: [replay](../replay.md).

## Replay of a whole cluster

| Name | Package | Measures |
|---|---|---|
| `BenchmarkRunScenarioLive` | `server/game` | building the world of the tests and running the scenario of `runScenario` (chat in each scope, a shield, an attack, a pickup, a run of movement, a new document of the config, a character that crosses to another zone, 90 ticks of each of 6 engines), with a memory event log |
| `BenchmarkReplayCluster` | `server/game` | building the same world and replaying the records of that scenario with `replay.Run` |

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| RunScenarioLive | 992,305 | 855,472 | 11,320 |
| ReplayCluster | 1,227,060 | 874,565 | 12,530 |

- A replay runs 672 engine ticks, so about 1.8 µs for each, the building of the cluster included. The same benchmark on the commit before the world was made by requests (the coordinator engine asks for the world, and the world for its zones and zonelayers, so the cluster has 7 engines where it had 6) ran 623 engine ticks: 869 µs the run and 1,093 µs the replay, measured the same day. The runs of one benchmark differ by up to 15 percent from one run of three to the next on this machine, so the 10 percent more is about the size of that spread.
- A replay costs about 1.2 times what the run does. It reads each log twice, once to index it and once to run it, works out every hash it compares, and hashes each message that it keeps for comparing; and it does not run the scenario's own sends. The plan (`RecoveryPlanFrom`) reads each log once more, to work out the line, which this benchmark does not include.
- `stepping.Step` (once `Cluster.Step`) takes a copy of the list of engines under a lock, since an engine can be removed while it runs: about 100 allocations and 1.8 percent more for the scenario (777,380 to 791,455 ns/op).
- A replay has an `AckBoard`, so its engines hold their sent messages and send the unacked ones again (`resendUnackedMessages`), as a live engine does. Before and after: ReplayCluster 990,000 and 1,001,325 ns/op, ReplayFromTheCheckpoints 7.96 and 7.85 ms/op, ReplayFromTickZero 59.5 and 59.4 ms/op: no change on these worlds, which hold few messages that wait for an ack. A world with many messages that wait has not been measured.
- What it keeps no longer grows with the log: a copy of an engine goes as soon as its last read is made, and the logs are read a tick at a time. A scenario this short cannot show it; `TestACopyIsLetGoOfAsSoonAsItsLastReadIsMade` shows that no copy is kept when the run is over.
- What still grows with the log is the index: about 40 bytes for each message that was taken, a count for each copy that was read, and the keys of the messages that came back. For a log of 10 million messages that is about 400 MB; keeping the index on disk, or only for the messages that a sender and a receiver of the same tick range share, would take it off, when a log that long has to be replayed whole.

## Recovery

All of them are in `server/game` and run on the files of a world that ran the scenario of `testkit.RunScenario` and then 3,000 ticks more (3,100 ticks of each of 7 engines: the coordinator engine, the world, 2 zones and 3 zonelayers), with a checkpoint every 500 ticks, written to disk by an `eventlog.FileLog` and a `checkpoint.FileWriter`.

| Name | Measures |
|---|---|
| `BenchmarkRecoveryLineWithTheCacheAtTheEndOfTheLogs` | `replay.MakePlan` (the line and the start of each engine) with the cache that the log wrote when it was closed: no log has a tick after what it says is safe, the best case |
| `BenchmarkRecoveryLineWithNoCache` | `replay.MakePlan` with no cache: every log is read from tick 1, the worst case |
| `BenchmarkReplayFromTickZero` | `replay.Run` on the files, each engine from tick 0 |
| `BenchmarkReplayFromTheCheckpoints` | `replay.Run` on the files, each engine from the start that `replay.MakePlan` chose |
| `BenchmarkRecover` | `replay.Recover`: the plan with its cache, the cut (nothing to cut), and the replay from the starts |

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| RecoveryLineWithTheCacheAtTheEndOfTheLogs | 5,547,300 | 13,516,400 | 27,352 |
| RecoveryLineWithNoCache | 18,688,900 | 43,502,900 | 184,756 |
| ReplayFromTickZero | 74,934,300 | 83,893,400 | 707,934 |
| ReplayFromTheCheckpoints | 9,748,530 | 14,155,100 | 72,888 |
| Recover | 19,182,500 | 28,254,900 | 103,818 |

The line and the choice of the start of each engine are one call now (`MakePlan`), so the first two rows hold the starts too: they read the logs after each checkpoint to find the reads of copies, which the line alone did not (it cost 1,901,000 and 7,062,000 ns/op before).

- A replay from the checkpoints costs 13 percent of one from tick 0: each engine runs the ticks after its checkpoint, at most 500, instead of 3,100, and the index reads the logs from there too.
- The cache takes the plan from 18.7 ms to 5.5 ms, to 30 percent of it. On the commit before the cluster made engines by requests the same two benchmarks ran 18.0 and 5.7 ms the same day, and `Recover` 18.6 ms against 19.2 ms now: the change adds nothing that shows. The 9.5 and 3.6 ms of the table before could not be reproduced, so the rise from them came earlier, and where is not known. What is left of the plan is opening the 7 logs, finding their last tick, and reading the logs after each checkpoint for the choice of the starts, which a cache of the line cannot take away.
- `Recover` costs about a quarter of the replay from tick 0, with the plan, the cache, the cut and the files of the checkpoints included. The part that grows with the length of the log is the replay; what the line costs follows the ticks after the cache, and what the starts cost follows the ticks after each checkpoint.
