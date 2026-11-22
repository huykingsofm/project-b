# Benchmarks of the replay runner

Conventions and how to run: [index](index.md). Behavior: [replay_runner](../replay_runner.md).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkRunScenarioLive` | `engine/replay/runner` | building the world of the tests and running the scenario of `runScenario` (chat in each scope, a shield, an attack, a pickup, a run of movement, a new version of the config, a character that crosses to another zone, 90 ticks of each of 6 engines), with a memory replay log |
| `BenchmarkReplayCluster` | `engine/replay/runner` | building the same world and replaying the records of that scenario with `runner.Run` |

## Results

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| RunScenarioLive | 638,000 | 807,000 | 8,263 |
| ReplayCluster | 675,000 | 718,000 | 7,444 |

- A replay runs 456 engine ticks, so about 1.4 µs for each, the building of the cluster included.
- A replay costs about what the run costs, 6 percent more: it works out every hash it compares and writes nothing, and it does not run the scenario's own sends.
