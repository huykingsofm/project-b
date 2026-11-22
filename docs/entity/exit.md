# Exit

`zone.ExitEntity`. Held in the state of a zone (built from the exits of its map when the zone state is made); every zonelayer of the zone has them.

## Fields

| Field | Type | Meaning |
|---|---|---|
| `EntityID` | `EntityID` | made of the ID of the zone and the number it counted |
| `Area` | `Rect` | the exit, both ends included, in units |
| `ToZoneID` | `sim.StateID` | the ID of the zone it leads to |
| `Arrival` | `Vec` | where a character comes out in that zone, in units |

## Behavior

- `Position`: the middle of `Area`, rounded towards `Min`.
- `Radius`: the longer half-side of `Area`.
- `BoundingBox`: `Area`.
- `Contains(v)`: `v` is inside `Area`.
- Hash: the ID, the bounding box, then `ToZoneID`, `Arrival`.

## Source

Defined on the map as `tilemap.ExitDefinition{Min, Max Tile; ToZoneID; Arrival Tile}`; `TileMap.Exits()` returns `tilemap.Exit` values, in units, and the zone makes an `ExitEntity` of each with an ID of its own.

## Used by

- [`ZoneLayerCharacterExitToAction`](../action/zonelayer.md#zonelayercharacterexittoaction): reads the exit by `ExitEntityByID` from the published state of the zone.
