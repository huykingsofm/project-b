# Benchmarks of the checkpoint

Conventions and how to run: [index](index.md). Behavior: [checkpoint](../checkpoint.md).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkCheckpointZoneLayer`, `BenchmarkRestoreZoneLayer` | `engine` | writing, and making an engine from, the checkpoint of a zonelayer with 150 characters, 150 items on the ground, and 150 actions that wait for a later tick |
| `BenchmarkCheckpointWorld`, `BenchmarkRestoreWorld` | `engine` | the same for the world with 2,000 characters that have a full inventory of 20 slots, and the 1,000 private messages it keeps |
| `BenchmarkHashZoneLayerWithWhatItWaitsFor`, `BenchmarkHashZoneLayerState` | `engine` | the hash of the engine (its state and what it waits for), and of the state alone |
| `BenchmarkHashWorld` | `engine` | the hash of the engine of that world |
| `BenchmarkCloneZoneLayer`, `BenchmarkCloneWorld` | `engine` | the clone that every engine publishes at the end of every tick, for comparison |

## Results

| Benchmark | ns/op | B/op | allocs/op | Size written |
|---|---|---|---|---|
| CheckpointZoneLayer | 39,300 | 35,600 | 17 | 7.6 KB |
| RestoreZoneLayer | 148,700 | 169,900 | 1,487 | |
| CloneZoneLayer | 36,000 | 83,700 | 623 | |
| HashZoneLayerWithWhatItWaitsFor | 22,800 | 13,500 | 311 | |
| HashZoneLayerState | 11,400 | 12,000 | 301 | |
| CheckpointWorld | 446,000 | 554,400 | 25 | 122 KB |
| RestoreWorld | 1,264,000 | 1,548,800 | 8,618 | |
| CloneWorld | 367,000 | 713,700 | 4,028 | |
| HashWorld | 97,700 | 160 | 7 | |

## Reading

- **Nothing is too slow.** One checkpoint of every engine of a cluster of 2,000 players (13 zonelayers, the world) costs about 1 ms of work, spread over engines that each write at their own tick, once for each period of the checkpoint.
- **Against the tick of 45 ms:** a zonelayer takes 0.09 percent of it, the world 1 percent, and only on the tick it writes at.
- **The checkpoint costs what a clone costs.** Every engine already clones its state at the end of every tick to publish it: 36 µs for a zonelayer, 367 µs and 714 KB for the world. A checkpoint is about as much work as one such clone and happens a period of ticks apart, so the clone is the cost to look at, not the checkpoint. The world clones the inventory of every character at every tick; the owner of the published copy can make it cheaper (copy only what changed, or share the inventories that did not).
- **The biggest single piece is the world:** 122 KB and 0.45 ms, proportional to the players (their inventories). Taken on the goroutine of the world it is 1 percent of a tick; the plan writes it outside the goroutine, from a copy taken at the end of the tick, and then the tick pays the copy and not the encoding.
- **Writing is 190 to 275 MB/s:** about 65 ns for each value, in line with small numbers in varints and one call through an interface for each entity and action. A buffer of the size that is written would take 10 percent off the world and nothing off a zonelayer (tried: a zonelayer got slower, because a large buffer must be cleared); it is not worth doing before a measured need.
- **Reading is 3 to 4 times slower than writing** and allocates a value for each entity, character and action. It happens once, at startup: about 3 ms for the whole cluster. Making each inventory with the room for its slots took the world from 1.75 to 1.26 ms and its allocations from 18,600 to 8,600.
- **The hash:** what the engine waits for and what only it knows add 11 µs to a zonelayer (the second is a few hundred nanoseconds and 4 allocations), and the hash is taken every 20 ticks. The hash of the state allocates once for each entity (301): the hasher is given to each entity through an interface and goes to the heap. This is older than the checkpoint; the owner of the hash of the state can take it off.
- **Storage, which the archive job will see:** a set of checkpoints of this cluster is about 220 KB. Nothing is deleted, so a period of 600 ticks at 10 ticks a second is about 13 MB an hour, 0.3 GB a day, before any compression.
