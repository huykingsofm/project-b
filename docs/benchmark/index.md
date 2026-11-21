# Benchmarks

The results of the benchmarks, as of the code now. Rule: [R13](../../plan/rule/r13-benchmarks.md).

## Files

| File | Benchmarks | Code |
|---|---|---|
| [move.md](move.md) | the path of a move: `TileMap.Reach` and `State.MoveEntity` | `sim/tilemap`, `sim/state/zonelayer` |
| [logging.md](logging.md) | a log call and the encoding of a batch | `shared/logging` |

## Run

- Command: `go test -run xxx -bench <pattern> -benchmem -count 3 <packages>`, from `server/internal/services/game/sim` (`server/internal/shared/logging` for logging).
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
