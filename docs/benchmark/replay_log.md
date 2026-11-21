# Benchmarks of the replay log in files

Conventions and how to run: [index](index.md). Behavior: [replay_log](../replay_log.md).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkEncodeRecord/<record>` | `engine/replay` | building the bytes of one record: the part of recording done by the goroutine of the engine, with no file and no writer |
| `BenchmarkDecodeRecord/<record>` | `engine/replay` | reading one record back |
| `BenchmarkFileTickHot` | `engine/replay` | a tick of 20 records after another with nothing between, to a temporary directory: the least a tick can cost, with everything in the cache |
| `BenchmarkFileTickPaced/records=N` | `engine/replay` | what an engine pays: 3,000 ticks of 20, 100 or 1,000 records with a millisecond between them, each tick timed from the engine's side |
| `BenchmarkFileTickSaturated` | `engine/replay` | ticks of 100 records with no pause, faster than an engine can tick, so the goroutine that writes falls behind |
| `BenchmarkFileTickManyEngines` | `engine/replay` | 12 engines on 12 goroutines, each with its own log, ticks of 100 records with no pause |

- A tick is a config read, a stable read, a hash, 5 commands and the rest inbound messages. The hash is a constant: taking a hash is the cost of the engine.
- Segments of 64 MiB, a backlog of 64 batches and a sync every second, in a temporary directory; the numbers are of WSL2 with its page cache, so they say nothing of the speed of a disk.
- `BenchmarkFileTickPaced` has a fixed workload: its `ns/op` and `B/op` are of the whole run and mean nothing; its metrics are in ns/tick.
- `stalls` is how many `Commit` calls had to wait for the goroutine that writes.

## Results

Building and reading a record, ns/op:

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| EncodeRecord, inbound with 2 actions | 68.5 | 0 | 0 |
| EncodeRecord, command: move | 31.9 | 0 | 0 |
| EncodeRecord, command: chat | 33.6 | 0 | 0 |
| EncodeRecord, config read | 7.8 | 0 | 0 |
| EncodeRecord, hash | 7.4 | 0 | 0 |
| EncodeRecord, stable read | 11.3 | 0 | 0 |
| DecodeRecord, inbound with 2 actions | 111 | 96 | 3 |
| DecodeRecord, command: move | 67.7 | 64 | 2 |
| DecodeRecord, command: chat | 101 | 120 | 3 |
| DecodeRecord, config read | 31.8 | 48 | 1 |
| DecodeRecord, hash | 30.4 | 48 | 1 |
| DecodeRecord, stable read | 37.3 | 48 | 1 |

A tick of the engine, as the engine sees it:

| Benchmark | mean | p50 | p99 | worst | stalls |
|---|---|---|---|---|---|
| FileTickHot, 20 records | 2.5 µs | | | | |
| FileTickPaced, 20 records | 9.4 µs | 8.0 µs | 35 µs | 0.46 ms | 0 |
| FileTickPaced, 100 records | 39.8 µs | 40.4 µs | 69 µs | 0.50 ms | 0 |
| FileTickPaced, 1,000 records | 182 µs | 190 µs | 258 µs | 0.54 ms | 0 |
| FileTickSaturated, 100 records | | 11.6 µs | 29 µs | 66 ms | 3 |
| FileTickManyEngines, 100 records | 2.8 µs per tick of any engine | | | | about 6,300 in all |

- `FileTickHot` has 0 allocations; it is a lower bound, 3 to 4 times better per record than the paced ticks, which find the cache cold and the goroutine that writes asleep.
- A tick of an engine costs 0.2 to 0.4 µs for each record, and under 1 percent of a tick of 45 ms even with 1,000 records.
- `FileTickSaturated` shows a tail: about 66 ms in the worst case, more than the 45 ms of a tick, and 39 ms to close. The number of stalls is the number of segments made (140 MB in 64 MiB segments): the goroutine that writes syncs a segment that it closes, which takes tens of milliseconds for 64 MiB of dirty pages, and while it does the backlog fills. With the sync of every second turned off the worst case did not change. It needs a load far beyond an engine (about 75,000 ticks a second against 50), or a disk that syncs very slowly; moving the sync of a closed segment to another goroutine would take it off the path.
