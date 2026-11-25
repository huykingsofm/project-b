# Benchmarks of the recovery

Conventions and how to run: [index](index.md). Behavior: [recovery](../recovery.md).

## Benchmarks

All of them are in `server/game` and run on the files of a world that ran the scenario of `testkit.RunScenario` and then 3,000 ticks more (3,100 ticks of each of 6 engines), with a checkpoint every 500 ticks, written to disk by a `replay.FileLog` and a `checkpoint.FileWriter`.

| Name | Measures |
|---|---|
| `BenchmarkRecoveryLineWithTheCacheAtTheEndOfTheLogs` | `recovery.RecoveryLine` with the cache that the log wrote when it was closed: no log has a tick after what it says is safe, the best case |
| `BenchmarkRecoveryLineWithNoCache` | `recovery.RecoveryLine` with no cache: every log is read from tick 1, the worst case |
| `BenchmarkReplayFromTickZero` | `runner.Run` on the files, each engine from tick 0 |
| `BenchmarkReplayFromTheCheckpoints` | `runner.Run` on the files, each engine from the start that `recovery.Recover` chose (`Result.Starts`) |
| `BenchmarkRecover` | `recovery.Recover`: the line with its cache, the cut (nothing to cut), the starts, and the replay from them |

## Results

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| RecoveryLineWithTheCacheAtTheEndOfTheLogs | 1,901,000 | 5,800,000 | 5,614 |
| RecoveryLineWithNoCache | 7,062,000 | 20,178,000 | 101,010 |
| ReplayFromTickZero | 35,184,000 | 59,058,000 | 520,975 |
| ReplayFromTheCheckpoints | 5,657,000 | 12,200,000 | 50,283 |
| Recover | 12,403,000 | 22,183,000 | 69,834 |

- A replay from the checkpoints costs 16 percent of one from tick 0: each engine runs the ticks after its checkpoint, at most 500, instead of 3,100, and the index reads the logs from there too.
- The cache takes the line from 7.1 ms to 1.9 ms, 27 percent. What is left of it is opening the 6 logs and finding their last tick, which a cache cannot take off. A cache that is up to a period behind costs between the two.
- `Recover` costs about a third of the replay from tick 0, with the line, the cache, the starts and the files of the checkpoints included. The part that grows with the length of the log is the replay; what the line costs follows the ticks after the cache, and what the starts cost follows the ticks after each checkpoint.
