# Benchmarks of the checkpoint

Conventions and how to run: [index](index.md). Behavior: [checkpoint](../checkpoint.md).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkCheckpointZoneLayer` | `engine` | writing the checkpoint of a zonelayer with 150 characters, 150 items on the ground, and 150 actions that wait for a later tick |
| `BenchmarkRestoreZoneLayer` | `engine` | making an engine of that zonelayer from its checkpoint |
| `BenchmarkHashZoneLayerWithWhatItWaitsFor` | `engine` | the hash of the engine: its state and what it waits for |
| `BenchmarkHashZoneLayerState` | `engine` | the hash of the state alone, for comparison |

## Results

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| CheckpointZoneLayer | 39,300 | 35,600 | 17 |
| RestoreZoneLayer | 148,700 | 169,900 | 1,487 |
| HashZoneLayerWithWhatItWaitsFor | 22,400 | 13,400 | 307 |
| HashZoneLayerState | 11,400 | 12,000 | 301 |

- The checkpoint is 7.6 KB: about 190 MB/s to write and 50 MB/s to read.
- Writing 39 µs is 0.09 percent of a tick of 45 ms. It makes 17 allocations, the buffer growing; a buffer kept from one checkpoint to the next would take them off.
- Reading allocates about one value for each entity, character and action (1,487): the entities are held in an interface, and the containers grow. A restore is rare, so it is left.
- What the engine waits for adds 11 µs and 6 allocations to the hash, and the hash is taken every 20 ticks: it costs nothing a tick would notice.
- The hash of the state alone allocates once for each entity (301): the hasher is given to each entity through an interface and goes to the heap. This is older than the checkpoint; the owner of the hash of the state can take it off by hashing an entity without passing the hasher by pointer through the interface.
