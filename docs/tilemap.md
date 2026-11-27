# TileMap

`sim/tilemap`. The map of a zone: never changes once made; every copy of the zone state shares it. Positions and radii are in units; a tile is 1024 units wide and high, a detail private to the package.

## Storage

- One bit for each tile, row by row, in a [`bitset.Array`](bitset.md); `width * height` bits.
- A row of 4 or more tiles of a body is read with `AnyInRange`; a column, or a row of fewer tiles, bit by bit.
- Compared with a `[]bool` in [the benchmarks of a move](benchmark/move.md#tile-storage).

## Construction

`New(width, height, spawn, walls) (*TileMap, error)`; sizes and coordinates in tiles. A map has no exit: the world puts exits in a zone while the game runs ([exit](entity/exit.md)). A tile is `TileSize` units (1024), the same on the server and the client, and it is never changed. Returns an error that wraps `ErrInvalid` and says what is wrong when the map is not well formed:

| Rule | |
|---|---|
| size | at least one tile each way, at most 2^20 tiles on a side and 4096 * 4096 tiles in all |
| walls | inside the map |
| spawn | inside the map, not a wall |

## Methods

| Method | Result |
|---|---|
| `SpawnPosition()` | the middle of the spawn tile |
| `Free(point)` | the point is in the map and not on a wall |
| `FreeForBody(position, radius)` | every tile the body (a square reaching `radius` from `position` on every side) touches is in the map and not a wall; a radius of 0 is a point; a negative radius fits nowhere |
| `Reach(from, dir, distance, radius)` | see below |
| `Holds(area)` | the area, in units, is not empty and is in the map |

`tilemap.AreaOf(min, max Tile)` is the area in units of the tiles from `min` to `max`, both included, and `Tile.Center()` the position at the middle of a tile.

## Reach

| | |
|---|---|
| Result | the farthest position along `dir`, up to `distance` units, at which the body still fits |
| Path | the whole path is followed: the front of the body is looked at each time it enters a new row or column of tiles, however far `distance` is |
| Stops | at the last unit before the body would touch a tile that is a wall or outside the map |
| Diagonal | crossing a corner needs both tiles beside the corner free |
| Does not fit at `from` | returns `from` |
| Cost | [benchmarks](benchmark/move.md) |

## Written and copied

| Call | What it does |
|---|---|
| `m.Encode(w)` | writes the size, the spawn, the walls (as the words of the bit array) and the exits; the zone writes its map with its state, in its checkpoint and in the record that begins its log |
| `tilemap.Decode(r, version)` | reads what `Encode` wrote; a size that its words do not fit is an error |
| `m.Clone()` | a copy that shares nothing with the map: the map never changes and the engines share one, but a command or an action that holds one is copied like anything that holds a reference (R29) |

