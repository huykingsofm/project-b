# Exit

`zone/entity.ExitEntity`. Implements `state.Entity`. Held in the state of a zone, put there while the game runs (`zone.State.AddExit`, by the world: [engines](../engine_lifecycle.md)); every zonelayer of the zone reads them from the published zone.

## Data and methods

The fields are private; the exit is a value, made with `NewExitEntity(id, area, toZoneID, arrival)`.

| Method | Returns | Meaning |
|---|---|---|
| `ID()` | `EntityID` | the number the zone counted |
| `KindID()` | `KindID` | `ExitKindID` |
| `Area()` | `Rect` | the exit, both ends included, in units |
| `ToZoneID()` | `sim.StateID` | the ID of the zone it leads to |
| `Arrival()` | `Vec` | where a character comes out in that zone, in units |
| `Position()` | `Vec` | the middle of the area, rounded towards its `Min` |
| `Radius()` | `int32` | the longer half-side of the area |
| `BoundingBox()` | `Rect` | the area |
| `Contains(v)` | `bool` | `v` is inside the area |
| `MovedTo(pos)` | `Entity` | the same exit with its middle at `pos` |

- Hash: the ID, the bounding box, then `ToZoneID()`, `Arrival()`.
- Written as the number the zone counted (not the whole ID), the area, the zone it leads to and the arrival.

## Source

The map has no exit. The world joins two zones with a command (`WorldZoneExitAddCommand{FromZoneID, Area, ToZoneID, Arrival}`: one way; both ways are two commands), which the world sends the zone as `ZoneExitAddAction`; the zone adds the exit (`AddExit`: the area, in units, must be in its map and not empty, and the target must be a zone) with the next number it counts, and tells the clients (`event.ExitAdded`). The area and the arrival are in units: a tile is `tilemap.TileSize` (1024) units, which is never changed (`tilemap.AreaOf`, `Tile.Center` turn tiles into units).

## Used by

- [`ZoneLayerCharacterExitToAction`](../action/zonelayer.md#zonelayercharacterexittoaction): reads the exit by `ExitEntityByID` from the published state of the zone.
