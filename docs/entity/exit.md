# Exit

`entity.ExitEntity`. Implements `Entity`. Held in the state of a zone (built from the exits of its map when the zone state is made); every zonelayer of the zone has them.

## Fields

| Field | Type | Meaning |
|---|---|---|
| `EntityID` | `EntityID` | ID in the zone |
| `Area` | `Rect` | the exit, both ends included, in units |
| `ToZoneID` | `ZoneID` | the zone it leads to |
| `Arrival` | `Vec` | where a character comes out in that zone, in units |

## Behavior

- `Position`: the middle of `Area`, rounded towards `Min`.
- `Radius`: the longer half-side of `Area`.
- `BoundingBox`: `Area`.
- `Contains(v)`: `v` is inside `Area`.
- `MovedTo`: moves the whole `Area`; `Arrival` stays.
- Hash: common part, then `ToZoneID`, `Arrival`.

## Source

Defined on the map as `tilemap.ExitDefinition{Min, Max Tile; ToZoneID; Arrival Tile}`; `TileMap.Exits()` returns the entities, in units.

## Used by

- [`ZoneLayerCharacterExitToAction`](../action/zonelayer.md#zonelayercharacterexittoaction): reads the exit by `ExitEntityByID` from the published state of the zone.
