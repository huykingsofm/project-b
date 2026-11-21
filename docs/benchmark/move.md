# Benchmarks of a move

Conventions and how to run: [index](index.md). Behavior: [`TileMap.Reach`](../tilemap.md#reach) and [`ZoneLayerCharacterMoveAction`](../action/zonelayer.md#zonelayercharactermoveaction).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkReach/radius R/a step of a quarter tile` | `sim/tilemap` | one `Reach` of 256 units east, in a map of 200 by 200 tiles |
| `BenchmarkReach/radius R/diagonal, 10 tiles` | `sim/tilemap` | one `Reach` of 10 tiles south-east |
| `BenchmarkReach/radius R/the whole map` | `sim/tilemap` | one `Reach` of 2 billion units east: it ends at the edge of the map, 200 tiles away |
| `BenchmarkMoveEntity/radius R/a character's step of a tick` | `sim/state/zonelayer` | one `MoveEntity` of 250 units, east and then west, with the events taken |

- `R` is the radius of the body, in units: 0, 256, 511 (a body that fills a tile), 2048 (a boss five tiles wide).

## Results

| Benchmark | radius | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| Reach, a step of a quarter tile | 0 | 18.0 | 0 | 0 |
| Reach, a step of a quarter tile | 256 | 21.1 | 0 | 0 |
| Reach, a step of a quarter tile | 511 | 30.8 | 0 | 0 |
| Reach, a step of a quarter tile | 2048 | 34.1 | 0 | 0 |
| Reach, diagonal, 10 tiles | 0 | 196 | 0 | 0 |
| Reach, diagonal, 10 tiles | 256 | 204 | 0 | 0 |
| Reach, diagonal, 10 tiles | 511 | 205 | 0 | 0 |
| Reach, diagonal, 10 tiles | 2048 | 263 | 0 | 0 |
| Reach, the whole map | 0 | 2395 | 0 | 0 |
| Reach, the whole map | 256 | 2406 | 0 | 0 |
| Reach, the whole map | 511 | 2458 | 0 | 0 |
| Reach, the whole map | 2048 | 3156 | 0 | 0 |
| MoveEntity, a step of a tick | 0 | 81.4 | 80 | 3 |
| MoveEntity, a step of a tick | 256 | 83.9 | 80 | 3 |
