# Benchmarks of the event log in files

Conventions and how to run: [index](index.md). Behavior: [eventlog](../eventlog.md).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkFileTickHot` | `platform/eventlog` | a tick of 20 records after another with nothing between, to a temporary directory: the least a tick can cost, with everything in the cache |
| `BenchmarkFileTickPaced/records=N` | `platform/eventlog` | what an engine pays: 3,000 ticks of 20, 100 or 1,000 records with a millisecond between them, each tick timed from the engine's side |
| `BenchmarkFileTickSaturated` | `platform/eventlog` | ticks of 100 records with no pause, faster than an engine can tick, so the goroutine that writes falls behind |
| `BenchmarkFileTickManyEngines` | `platform/eventlog` | 12 engines on 12 goroutines, each with its own log, ticks of 100 records with no pause |

- A tick is two stable reads, a hash, 5 commands and the rest inbound messages. The hash is a constant: taking a hash is the cost of the engine.
- Segments of 64 MiB, a backlog of 64 batches and a sync every second, in a temporary directory; the numbers are of WSL2 with its page cache, so they say nothing of the speed of a disk.
- `BenchmarkFileTickPaced` has a fixed workload: its `ns/op` and `B/op` are of the whole run and mean nothing; its metrics are in ns/tick.

## Results

A tick of the engine, as the engine sees it:

| Benchmark | mean | p50 | p99 | worst |
|---|---|---|---|---|
| FileTickHot, 20 records | 3.3 µs | | | |
| FileTickPaced, 20 records | 17.4 µs | 10.9 µs | 51 µs | 0.46 ms |
| FileTickPaced, 100 records | 48.5 µs | 48.8 µs | 83 µs | 0.49 ms |
| FileTickPaced, 1,000 records | 232 µs | 236 µs | 357 µs | 0.85 ms |
| FileTickSaturated, 100 records | | 14.1 µs | 37 µs | 47 ms |
| FileTickManyEngines, 100 records | 4.9 µs per tick of any engine | | | |

- `FileTickHot` has about 1 allocation for each record (19 of 292 bytes for a tick of 20): the body of a record is a value held in an interface, and an engine makes one for each `Append`. It is a lower bound, 2 to 4 times better per record than the paced ticks, which find the cache cold and the goroutine that writes asleep.
- A tick of an engine costs 0.2 to 0.9 µs for each record, about 10 percent more than when each kind of record had a method of its own on the log (the price of the allocation above, 20 to 30 ns for a record), and under 1 percent of a tick of 45 ms even with 1,000 records.
- `FileTickSaturated` shows a tail: about 41 ms in the worst case, close to the 45 ms of a tick, and 8 ms to close. The goroutine that writes syncs a segment that it closes, which takes tens of milliseconds for 64 MiB of dirty pages, and while it does the backlog fills. It needs a load far beyond an engine (about 60,000 ticks a second against 50), or a disk that syncs very slowly; moving the sync of a closed segment to another goroutine would take it off the path.
