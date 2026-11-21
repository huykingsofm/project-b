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
| Reach, a step of a quarter tile | 0 | 17.7 | 0 | 0 |
| Reach, a step of a quarter tile | 256 | 21.1 | 0 | 0 |
| Reach, a step of a quarter tile | 511 | 29.8 | 0 | 0 |
| Reach, a step of a quarter tile | 2048 | 39.2 | 0 | 0 |
| Reach, diagonal, 10 tiles | 0 | 196 | 0 | 0 |
| Reach, diagonal, 10 tiles | 256 | 212 | 0 | 0 |
| Reach, diagonal, 10 tiles | 511 | 217 | 0 | 0 |
| Reach, diagonal, 10 tiles | 2048 | 290 | 0 | 0 |
| Reach, the whole map | 0 | 2408 | 0 | 0 |
| Reach, the whole map | 256 | 2394 | 0 | 0 |
| Reach, the whole map | 511 | 2457 | 0 | 0 |
| Reach, the whole map | 2048 | 3347 | 0 | 0 |
| MoveEntity, a step of a tick | 0 | 82.4 | 80 | 3 |
| MoveEntity, a step of a tick | 256 | 85.0 | 80 | 3 |
| Reach, a step of a quarter tile, map of 4096 by 4096 | 256 | 25.1 | 0 | 0 |
| Reach, diagonal, 10 tiles, map of 4096 by 4096 | 256 | 231 | 0 | 0 |
| Reach, a step of a quarter tile, map of 4096 by 4096 | 2048 | 48.2 | 0 | 0 |
| Reach, diagonal, 10 tiles, map of 4096 by 4096 | 2048 | 303 | 0 | 0 |

## Tile storage

`TileMap` keeps one bit for each tile in a `bitset.Array` ([bitset](../bitset.md)). The same benchmarks with a `[]bool` (one byte each), in ns/op:

| Benchmark | radius | map | `[]bool` | bitset |
|---|---|---|---|---|
| Reach, a step of a quarter tile | 0 | 200 by 200 | 17.9 | 17.7 |
| Reach, diagonal, 10 tiles | 0 | 200 by 200 | 190 | 196 |
| Reach, the whole map | 0 | 200 by 200 | 2383 | 2408 |
| Reach, a step of a quarter tile | 256 | 200 by 200 | 21.1 | 21.1 |
| Reach, diagonal, 10 tiles | 256 | 200 by 200 | 204 | 212 |
| Reach, the whole map | 256 | 200 by 200 | 2391 | 2394 |
| Reach, a step of a quarter tile | 511 | 200 by 200 | 29.4 | 29.8 |
| Reach, diagonal, 10 tiles | 511 | 200 by 200 | 204 | 217 |
| Reach, the whole map | 511 | 200 by 200 | 2427 | 2457 |
| Reach, a step of a quarter tile | 2048 | 200 by 200 | 33.2 | 39.2 |
| Reach, diagonal, 10 tiles | 2048 | 200 by 200 | 257 | 290 |
| Reach, the whole map | 2048 | 200 by 200 | 3112 | 3347 |
| Reach, a step of a quarter tile | 256 | 4096 by 4096 | 45.3 | 25.1 |
| Reach, diagonal, 10 tiles | 256 | 4096 by 4096 | 283 | 231 |
| Reach, a step of a quarter tile | 2048 | 4096 by 4096 | 68.5 | 48.2 |
| Reach, diagonal, 10 tiles | 2048 | 4096 by 4096 | 370 | 303 |
