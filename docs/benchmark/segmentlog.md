# Benchmarks of the segment log

Conventions and how to run: [index](index.md). Behavior: [segmentlog](../segmentlog.md).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkAppendOnly` | `shared/segmentlog` | what the caller pays: `Append` of a record of 100 bytes, with the `Commit` and the writing left out of the time |
| `BenchmarkAppendAndCommit/records per tick=N` | `shared/segmentlog` | the throughput of the whole path: records of 100 bytes, `N` for each `Commit`, written to a temporary directory with segments of 64 MiB, a backlog of 64 batches and a sync every second; per record, including the waits when the goroutine that writes is behind (`stalls` counts them) |
| `BenchmarkRead` | `shared/segmentlog` | `Reader.Next` over 65,536 records of 100 bytes |

## Results

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| AppendOnly | 20.9 | 0 | 0 |
| AppendAndCommit, 1 record per tick | 1645 | 0 | 0 |
| AppendAndCommit, 100 records per tick | 210 | 0 | 0 |
| AppendAndCommit, 1000 records per tick | 222 | 0 | 0 |
| Read | 47.9 | 9 | 1 |

- What the caller of `Append` pays is `AppendOnly`, about 21 ns. `AppendAndCommit` is the speed of the goroutine that writes: nearly every `Commit` of it waits (stalls are about every batch at 1 record per tick), so its time is the time of `write` calls to the file, not of the caller.
- At 1 record per tick the time is the cost of one `write` call for each batch.
