# Exit

`zone/entity.ExitEntity`. Implements `state.Entity`. Held in the state of a zone (built from the exits of its map when the zone state is made); every zonelayer of the zone has them.

## Data and methods

The fields are private; the exit is a value, made with `NewExitEntity(id, area, toZoneID, arrival)`.

| Method | Returns | Meaning |
|---|---|---|
| `ID()` | `EntityID` | made of the ID of the zone and the number it counted |
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

Defined on the map as `tilemap.ExitDefinition{Min, Max Tile; ToZoneID; Arrival Tile}`; `TileMap.Exits()` returns `tilemap.Exit` values, in units, and the zone makes an `ExitEntity` of each with an ID of its own.

## Used by

- [`ZoneLayerCharacterExitToAction`](../action/zonelayer.md#zonelayercharacterexittoaction): reads the exit by `ExitEntityByID` from the published state of the zone.
