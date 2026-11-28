# Benchmarks of a move

Conventions and how to run: [index](index.md). Behavior: [`TileMap.Reach`](../tilemap.md#reach) and [`ZoneLayerCharacterMoveAction`](../action/zonelayer.md#zonelayercharactermoveaction).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkReach/radius R/a step of a quarter tile` | `sim/tilemap` | one `Reach` of 256 units east, in a map of 200 by 200 tiles |
| `BenchmarkReach/radius R/diagonal, 10 tiles` | `sim/tilemap` | one `Reach` of 10 tiles south-east |
| `BenchmarkReach/radius R/the whole map` | `sim/tilemap` | one `Reach` of 2 billion units east: it ends at the edge of the map, 200 tiles away |
| `BenchmarkMoveEntity/radius R/a character's step of a tick` | `sim/state/zonelayer` | one `MoveEntity` of 250 units, east and then west, with the events taken |

- `BenchmarkReachLargeMap` (`sim/tilemap`): the same `Reach` from 4,096 random places of a map of 4096 by 4096 tiles, with radius 256 and 2048 (`a step of a quarter tile`, `diagonal, 10 tiles`).
- `R` is the radius of the body, in units: 0, 256, 511 (a body that fills a tile), 2048 (a boss five tiles wide).

## Results

| Benchmark | radius | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| Reach, a step of a quarter tile | 0 | 18.4 | 0 | 0 |
| Reach, a step of a quarter tile | 256 | 21.7 | 0 | 0 |
| Reach, a step of a quarter tile | 511 | 30.3 | 0 | 0 |
| Reach, a step of a quarter tile | 2048 | 32.6 | 0 | 0 |
| Reach, diagonal, 10 tiles | 0 | 205 | 0 | 0 |
| Reach, diagonal, 10 tiles | 256 | 218 | 0 | 0 |
| Reach, diagonal, 10 tiles | 511 | 218 | 0 | 0 |
| Reach, diagonal, 10 tiles | 2048 | 266 | 0 | 0 |
| Reach, the whole map | 0 | 2459 | 0 | 0 |
| Reach, the whole map | 256 | 2454 | 0 | 0 |
| Reach, the whole map | 511 | 2448 | 0 | 0 |
| Reach, the whole map | 2048 | 3272 | 0 | 0 |
| MoveEntity, a step of a tick | 0 | 83.1 | 80 | 3 |
| MoveEntity, a step of a tick | 256 | 89.1 | 80 | 3 |
| Reach, a step of a quarter tile, map of 4096 by 4096 | 256 | 22.7 | 0 | 0 |
| Reach, diagonal, 10 tiles, map of 4096 by 4096 | 256 | 224 | 0 | 0 |
| Reach, a step of a quarter tile, map of 4096 by 4096 | 2048 | 38.4 | 0 | 0 |
| Reach, diagonal, 10 tiles, map of 4096 by 4096 | 2048 | 279 | 0 | 0 |

## Tile storage

`TileMap` keeps one bit for each tile in a `bitset.Array` ([bitset](../shared.md#bitset)). The same benchmarks, in ns/op, with a `[]bool` (one byte each), with the bitset read one tile at a time, and with the bitset where a row of 4 or more tiles is read with `AnyInRange` (what the code does):

| Benchmark | radius | map | `[]bool` | bitset, a tile at a time | bitset, rows with `AnyInRange` |
|---|---|---|---|---|---|
| Reach, a step of a quarter tile | 0 | 200 by 200 | 17.9 | 18.1 | 18.4 |
| Reach, diagonal, 10 tiles | 0 | 200 by 200 | 190 | 197 | 205 |
| Reach, the whole map | 0 | 200 by 200 | 2383 | 2436 | 2459 |
| Reach, a step of a quarter tile | 256 | 200 by 200 | 21.1 | 21.1 | 21.7 |
| Reach, diagonal, 10 tiles | 256 | 200 by 200 | 204 | 216 | 218 |
| Reach, the whole map | 256 | 200 by 200 | 2391 | 2438 | 2454 |
| Reach, a step of a quarter tile | 511 | 200 by 200 | 29.4 | 29.9 | 30.3 |
| Reach, diagonal, 10 tiles | 511 | 200 by 200 | 204 | 216 | 218 |
| Reach, the whole map | 511 | 200 by 200 | 2427 | 2437 | 2448 |
| Reach, a step of a quarter tile | 2048 | 200 by 200 | 33.2 | 38.9 | 32.6 |
| Reach, diagonal, 10 tiles | 2048 | 200 by 200 | 257 | 288 | 266 |
| Reach, the whole map | 2048 | 200 by 200 | 3112 | 3329 | 3272 |
| Reach, a step of a quarter tile | 256 | 4096 by 4096 | 45.3 | 21.6 | 22.7 |
| Reach, diagonal, 10 tiles | 256 | 4096 by 4096 | 283 | 227 | 224 |
| Reach, a step of a quarter tile | 2048 | 4096 by 4096 | 68.5 | 41.3 | 38.4 |
| Reach, diagonal, 10 tiles | 2048 | 4096 by 4096 | 370 | 293 | 279 |
