# Benchmarks of the replay runner

Conventions and how to run: [index](index.md). Behavior: [replay_runner](../replay_runner.md).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkRunScenarioLive` | `platform/replay/runner` | building the world of the tests and running the scenario of `runScenario` (chat in each scope, a shield, an attack, a pickup, a run of movement, a new version of the config, a character that crosses to another zone, 90 ticks of each of 6 engines), with a memory replay log |
| `BenchmarkReplayCluster` | `platform/replay/runner` | building the same world and replaying the records of that scenario with `runner.Run` |

## Results

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| RunScenarioLive | 641,700 | 722,000 | 9,353 |
| ReplayCluster | 645,000 | 689,000 | 8,705 |

- A replay runs 477 engine ticks, so about 1.4 µs for each, the building of the cluster included.
- A replay costs about what the run does (0.5 percent more). It reads each log twice, once to index it and once to run it, works out every hash it compares, and hashes each message that it keeps for comparing; and it does not run the scenario's own sends.
- A replay from checkpoints and the recovery are in [recovery](recovery.md).
- What it keeps no longer grows with the log: a copy of an engine goes as soon as its last read is made, and the logs are read a tick at a time. A scenario this short cannot show it; `TestACopyIsLetGoOfAsSoonAsItsLastReadIsMade` shows that no copy is kept when the run is over.
- What still grows with the log is the index: about 40 bytes for each message that was taken, a count for each copy that was read, and the keys of the messages that came back. For a log of 10 million messages that is about 400 MB; keeping the index on disk, or only for the messages that a sender and a receiver of the same tick range share, would take it off, when a log that long has to be replayed whole.
