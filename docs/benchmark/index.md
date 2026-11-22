# Benchmarks

The results of the benchmarks, as of the code now. Rule: [R13](../../plan/rule/r13-benchmarks.md).

## Files

| File | Benchmarks | Code |
|---|---|---|
| [move.md](move.md) | the path of a move: `TileMap.Reach` and `State.MoveEntity` | `sim/tilemap`, `sim/state/zonelayer` |
| [logging.md](logging.md) | a log call and the encoding of a batch | `shared/logging` |
| [bitset.md](bitset.md) | `bitset.Array` against `[]bool` | `shared/bitset` |
| [segmentlog.md](segmentlog.md) | appending to and reading the segment log | `shared/segmentlog` |
| [replay_log.md](replay_log.md) | recording and reading the replay log in files | `engine/replay` |
| [plan.md](plan.md) | filling a plan and finding actions in it | `sim/action` |
| [codec.md](codec.md) | writing and reading a message with its actions | `sim/action` |

## Run

- Command: `go test -run xxx -bench <pattern> -benchmem -count 3 ./...`
- Directory to run it in:

| Benchmarks of | Directory |
|---|---|
| the simulation, the codec of messages | `server/internal/services/game/sim` |
| logging | `server/internal/shared/logging` |
| the bitset | `server/internal/shared/bitset` |
| the segment log | `server/internal/shared/segmentlog` |
| the replay log | `server/internal/services/game/engine/replay` |

- Record the median of the runs.

## Record

- Every file has the same sections: Benchmarks, Results.
- Benchmarks: name, the package it is in, what it measures.
- Results: the numbers of the latest run of every benchmark of the file, with `ns/op`, `B/op` and `allocs/op`.
- A change that could affect a benchmark runs it again and replaces the numbers in place. The history is in git.

## Environment

| Key | Value |
|---|---|
| CPU | AMD Ryzen 7 5800H, 12 threads |
| OS | Linux (WSL2) |
| Go | 1.27.4 |
