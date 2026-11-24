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
| `BenchmarkWriteFileZoneLayer`, `BenchmarkWriteFileWorld` | `platform/checkpoint` | writing the file of a checkpoint of 8.3 KB and of 122 KB, synced to the disk: what the writer pays for each besides encoding it |

## Results

The run of this table found the machine slower than the runs before it: the clone of a zonelayer, which no change touched, is 44 µs here and was 36.5 µs.

| Benchmark | ns/op | B/op | allocs/op | Size written |
|---|---|---|---|---|
| TakeCheckpointZoneLayer | 53,100 | 114,000 | 943 | |
| EncodeCheckpointZoneLayer | 28,100 | 34,300 | 16 | 8.5 KB |
| RestoreZoneLayer | 204,000 | 225,700 | 1,541 | |
| CloneZoneLayer | 44,300 | 98,800 | 632 | |
| HashZoneLayerWithWhatItWaitsFor | 17,000 | 14,500 | 309 | |
| HashZoneLayerState | 13,100 | 14,400 | 301 | |
| TakeCheckpointWorld | 208,000 | 582,600 | 2,021 | |
| EncodeCheckpointWorld | 447,000 | 596,800 | 27 | 125 KB |
| RestoreWorld | 870,000 | 851,500 | 3,095 | |
| CloneWorld | 207,000 | 582,600 | 2,018 | |
| HashWorld | 98,200 | 96 | 6 | |
| StepZoneLayer | 46,200 | 96,600 | 646 | |
| StepZoneLayerTakingACheckpoint | 68,400 | 112,900 | 893 | |
| StepWorld | 266,000 | 583,100 | 2,020 | |
| StepWorldTakingACheckpoint | 363,000 | 583,700 | 2,032 | |
| WriteFileZoneLayer | 2,630,000 | 1,600 | 22 | 8.3 KB |
| WriteFileWorld | 3,000,000 | 1,600 | 22 | 122 KB |

## Reading

- **The tick a checkpoint is taken at:** 22 µs more for the zonelayer (68 against 46 µs) and 97 µs more for the world (363 against 266 µs). Most of it is the hash of the engine, which the log has at that tick (17 µs and 98 µs alone); the rest is the clone of the scheduler (150 actions for the zonelayer: one allocation each), of the random numbers and of what the peer knows. The state is the clone that is published anyway. Once a period (600 ticks), so at most 0.2 percent of a tick of 45 ms on the tick it is taken at, and nothing on the others.
- **Taking a checkpoint between two ticks** (`Engine.Checkpoint`) is a clone of the state and of the rest: 53 µs for the zonelayer, 208 µs for the world, about what the clone of the state costs alone. It is done once, at the handover of a recovery, not on a tick.
- **Encoding is on the goroutine of the writer:** 28 µs and 8.5 KB for the zonelayer, 0.45 ms and 125 KB for the world, proportional to the players (their inventories); 0.06 and 1 percent of a tick of 45 ms, on no tick.
- **Writing a file is 2.6 to 3 ms, nearly all of it the two syncs** (the file, then the directory), on WSL2: the size changes little. One writer for a cluster of 2,000 players (14 engines) spends some 40 ms in a period of 600 ticks, off the ticks of the engines; the queue holds what comes while it waits.
- **A tick of the world is 266 µs with nothing sent to it,** most of it the clone it publishes (207 µs). The hash of the engine is worked out only at the ticks the log records it at (every 20 ticks, and at a checkpoint), so a tick pays it once in 20 on average.
- **The clone is the cost to look at, not the checkpoint.** Every engine clones its state at the end of every tick to publish it, and a checkpoint is about one such clone a period of ticks apart. The world clones the inventory of every character at every tick; the owner of the published copy can make it cheaper (copy only what changed, or share the inventories that did not).
- **Encoding is 280 to 300 MB/s:** about 65 ns for each value, in line with small numbers in varints and one call through an interface for each entity and action. A buffer of the size that is written would take 10 percent off the world and nothing off a zonelayer (tried: a zonelayer got slower, because a large buffer must be cleared); it is not worth doing before a measured need.
- **Reading is 4 to 7 times slower than encoding** and allocates a value for each entity, character and action. It happens once, at startup: about 3 ms for the whole cluster.
- **The hash:** what the engine waits for and what only it and its peer know add 4 µs to a zonelayer, and the hash is taken every 20 ticks. The scheduler hashes its ticks in the order it keeps them, with no sort. The hash of the state allocates once for each entity (301): the hasher is given to each entity through an interface and goes to the heap; the owner of the hash of the state can take it off.
- **Storage, which the archive job will see:** one checkpoint of every engine of this cluster is about 220 KB. Nothing is deleted, so a period of 600 ticks at 10 ticks a second is about 13 MB an hour, 0.3 GB a day, before any compression.
