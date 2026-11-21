# Benchmarks of logging

Conventions and how to run: [index](index.md). Behavior: [logging](../logging.md).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkGlobalCall` | `shared/logging` | one `logging.Info` with two pairs into a queue that is never full |
| `BenchmarkWithLoggerCall` | `shared/logging` | one `Info` with one pair on a logger made by `With` with one pair |
| `BenchmarkEncodePerEntry` | `shared/logging` | the writer consumer encoding a batch of 1,024 entries, per entry, to `io.Discard` |

## Results

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| GlobalCall | 188 | 144 | 1 |
| WithLoggerCall | 184 | 176 | 2 |
| EncodePerEntry | 269 | 0 | 0 |
