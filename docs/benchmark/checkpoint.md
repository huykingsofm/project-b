# Benchmarks of the checkpoint

Conventions and how to run: [index](index.md). Behavior: [checkpoint](../checkpoint.md).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkTakeCheckpointZoneLayer`, `BenchmarkEncodeCheckpointZoneLayer`, `BenchmarkRestoreZoneLayer` | `server/game` | taking the checkpoint of a zonelayer with 150 characters, 150 items on the ground and 150 actions that wait for a later tick between two ticks (`Engine.Checkpoint`: a clone of all of it), encoding it, and reading it and making an engine from it |
| `BenchmarkTakeCheckpointWorld`, `BenchmarkEncodeCheckpointWorld`, `BenchmarkRestoreWorld` | `server/game` | the same for the world with 2,000 characters that have a full inventory of 20 slots, and the 1,000 private messages it keeps |
| `BenchmarkHashZoneLayerWithWhatItWaitsFor`, `BenchmarkHashZoneLayerState` | `server/game` | the hash of the engine (its state and what it waits for), and of the state alone |
| `BenchmarkHashWorld` | `server/game` | the hash of the engine of that world |
| `BenchmarkCloneZoneLayer`, `BenchmarkCloneWorld` | `server/game` | the clone that every engine publishes at the end of every tick, for comparison |
| `BenchmarkStepZoneLayer`, `BenchmarkStepZoneLayerTakingACheckpoint` | `server/game` | a tick of that zonelayer with nothing sent to it, without a checkpoint and with one taken at the tick and handed to a writer that writes nothing: the difference is what the goroutine of the engine pays for a checkpoint |
| `BenchmarkStepWorld`, `BenchmarkStepWorldTakingACheckpoint` | `server/game` | the same for that world |
| `BenchmarkWriteFileZoneLayer`, `BenchmarkWriteFileWorld` | `platform/journal/checkpoint` | writing the file of a checkpoint of 8.3 KB and of 122 KB, synced to the disk: what the writer pays for each besides encoding it |

## Results

| Benchmark | ns/op | B/op | allocs/op | Size written |
|---|---|---|---|---|
| TakeCheckpointZoneLayer | 45,300 | 99,400 | 792 | |
| EncodeCheckpointZoneLayer | 25,600 | 34,300 | 16 | 7.2 KB |
| RestoreZoneLayer | 176,000 | 189,100 | 1,390 | |
| CloneZoneLayer | 39,700 | 86,600 | 632 | |
| HashZoneLayerWithWhatItWaitsFor | 14,200 | 12,100 | 308 | |
| HashZoneLayerState | 11,700 | 12,000 | 301 | |
| TakeCheckpointWorld | 188,000 | 582,600 | 2,021 | |
| EncodeCheckpointWorld | 442,000 | 596,800 | 27 | 123 KB |
| RestoreWorld | 869,000 | 851,700 | 3,096 | |
| CloneWorld | 191,000 | 582,500 | 2,018 | |
| HashWorld | 97,900 | 96 | 6 | |
| StepZoneLayer | 52,500 | 85,300 | 647 | |
| StepZoneLayerTakingACheckpoint | 65,000 | 99,300 | 888 | |
| StepWorld | 240,000 | 583,200 | 2,021 | |
| StepWorldTakingACheckpoint | 351,000 | 583,800 | 2,033 | |
| WriteFileZoneLayer | 8,434,000 | 1,541 | 19 | 8.3 KB |
| WriteFileWorld | 8,993,000 | 1,511 | 19 | 122 KB |

## Reading

- **The tick a checkpoint is taken at:** 13 µs more for the zonelayer (65 against 52 µs) and 111 µs more for the world (351 against 240 µs). Most of it is the hash of the engine, which the log has at that tick (14 µs and 98 µs alone); the rest is the clone of the scheduler (150 actions for the zonelayer: one allocation each), of the random numbers and of what the peer knows. The state is the clone that is published anyway. Once a period (600 ticks), so at most 0.2 percent of a tick of 45 ms on the tick it is taken at, and nothing on the others.
- **Taking a checkpoint between two ticks** (`Engine.Checkpoint`) is a clone of the state and of the rest: 45 µs for the zonelayer, 188 µs for the world, about what the clone of the state costs alone. It is done once, at the handover of a recovery, not on a tick.
- **Encoding is on the goroutine of the writer:** 26 µs and 7.2 KB for the zonelayer, 0.44 ms and 123 KB for the world, proportional to the players (their inventories); 0.06 and 1 percent of a tick of 45 ms, on no tick.
- **Writing a file is 8.4 to 9 ms, nearly all of it the two syncs** (the file, then the directory), on WSL2, where it was 2.6 to 3 ms in the run before: no code of the write changed, so the difference is the disk of the machine, and a run on another day can differ again. The size changes little. One writer for a cluster of 2,000 players (14 engines) spends some 120 ms in a period of 600 ticks, off the ticks of the engines; the queue holds what comes while it waits.
- **A tick of the world is 240 µs with nothing sent to it,** most of it the clone it publishes (191 µs). The hash of the engine is worked out only at the ticks the log records it at (every 20 ticks, and at a checkpoint), so a tick pays it once in 20 on average.
- **The clone is the cost to look at, not the checkpoint.** Every engine clones its state at the end of every tick to publish it, and a checkpoint is about one such clone a period of ticks apart. The world clones the inventory of every character at every tick; the owner of the published copy can make it cheaper (copy only what changed, or share the inventories that did not).
- **Encoding is about 280 MB/s:** about 65 ns for each value, in line with small numbers in varints and one call through an interface for each entity and action. A buffer of the size that is written would take 10 percent off the world and nothing off a zonelayer (tried: a zonelayer got slower, because a large buffer must be cleared); it is not worth doing before a measured need.
- **Reading is 4 to 7 times slower than encoding** and allocates a value for each entity, character and action. It happens once, at startup: about 3 ms for the whole cluster.
- **The hash:** what the engine waits for and what only it and its peer know add 2.5 µs to a zonelayer, and the hash is taken every 20 ticks. The scheduler hashes its ticks in the order it keeps them, with no sort. The hash of the state allocates once for each entity (301): the hasher is given to each entity through an interface and goes to the heap; the owner of the hash of the state can take it off.
- **Storage, which the archive job will see:** one checkpoint of every engine of this cluster is about 220 KB. Nothing is deleted, so a period of 600 ticks at 10 ticks a second is about 13 MB an hour, 0.3 GB a day, before any compression.
