# Benchmarks

The results of the benchmarks, as of the code now. Rule: [ways of working](../../plan/rule/ways-of-working.md).

## Files

| File | Benchmarks | Code |
|---|---|---|
| [move.md](move.md) | the path of a move: `TileMap.Reach` and `State.MoveEntity` | `sim/tilemap`, `sim/state/zonelayer` |
| [logging.md](logging.md) | a log call and the encoding of a batch | `shared/logging` |
| [bitset.md](bitset.md) | `bitset.Array` against `[]bool` | `shared/bitset` |
| [segmentlog.md](segmentlog.md) | appending to and reading the segment log | `shared/segmentlog` |
| [eventlog.md](eventlog.md) | recording and reading the event log in files | `platform/journal/eventlog` |
| [plan.md](plan.md) | filling a plan and finding actions in it | `platform/sim` |
| [codec.md](codec.md) | writing and reading a message with its actions | `platform/sim` |
| [replay.md](replay.md) | replaying a whole cluster run against its log, the recovery line with and without its cache, and the replay from tick 0 and from the checkpoints | `server/game` |
| [checkpoint.md](checkpoint.md) | writing, reading and hashing the checkpoint of an engine, taking it at a tick, and its file | `engine`, `platform/journal/checkpoint` |
| [actions.md](actions.md) | one `Apply` of each action of the world, a zone and a zonelayer | `server/game/sim/action` |
| [state_methods.md](state_methods.md) | each method of the three states that an action calls | `server/game/sim/state/*` |
| [router.md](router.md) | the send, the refusal and the return of a message, and the send from many engines | `platform/runtime/router` |
| [cores.md](cores.md) | zonelayers in parallel on 1, 2 and 4 CPUs, and the single-goroutine benchmarks on 1 and 2 | `server/game` |

## Run

- Command: `go test -run xxx -bench <pattern> -benchmem -count 3 ./...`; for the benchmarks on more than one core add `-cpu 1,2,4`, and `taskset -c 0,1` before `go test` to leave the program two cores
- Directory to run it in:

| Benchmarks of | Directory |
|---|---|
| the simulation, the codec of messages | `platform/sim`, `server/game/sim` |
| logging | `shared/logging` |
| the bitset | `shared/bitset` |
| the segment log | `shared/segmentlog` |
| the event log | `platform/journal/eventlog` |
| the checkpoint | `server/game`, `platform/journal/checkpoint` |
| the replay and the recovery, the zonelayers in parallel | `server/game` |
| the actions | `server/game/sim/action` |
| the methods of the states | `server/game/sim/state/world`, `zone`, `zonelayer` |
| the router | `platform/runtime/router` |

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
