# Benchmarks of the methods of the states

Conventions and how to run: [index](index.md). Behavior: [actions](../action/index.md) and [entities](../entity/entity.md).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkMethods/<method>` | `server/game/sim/state/zonelayer` | each method of a zonelayer that an action calls, on a zonelayer of 150 characters with an item on the ground by each |
| `BenchmarkMethods/<method>` | `server/game/sim/state/zone` | the same on a zone of 200 by 200 tiles with 10 exits and 4 zonelayers |
| `BenchmarkMethods/<method>` | `server/game/sim/state/world` | the same on a world of 2,000 characters with an inventory of 20 slots each, 3 zones and 6 zonelayers |

- A method that makes something is measured with the one that takes it away, in the same run (`AddCharacterEntity and RemoveCharacterEntity`), so that the state is the same at each run; the number is of both.
- A method that gives back one field, with no work (`CharacterCount`, `EntityCount`, `TileMap`, `ZoneIDs`, ...), is not here: the compiler takes the call out and the benchmark would say 0.5 ns. `AdvanceTick`, `Tick`, `Type`, `Clone`, `TakeEvents`, `Encode` and `Hash` are the ones that every engine calls on every state: the copy and the hash are in [checkpoint](checkpoint.md), the move in [move](move.md).
- `MoveEntity` is in [move](move.md).

## Results

A zonelayer:

| Method | ns/op | B/op | allocs/op |
|---|---|---|---|
| EntityByID | 7.3 | 0 | 0 |
| DamageEntity | 9.1 | 0 | 0 |
| CooldownUntil | 13.7 | 0 | 0 |
| ItemEntityByID | 13.8 | 0 | 0 |
| CharacterEntityByCharacterID | 23.6 | 0 | 0 |
| SetCooldown | 25.6 | 0 | 0 |
| SendMessage | 42.8 | 48 | 2 |
| CharacterByID | 53.5 | 0 | 0 |
| AddItemEntity and RemoveItemEntity | 299 | 136 | 5 |
| AddCharacterEntity and RemoveCharacterEntity | 538 | 152 | 5 |
| EntityIDsOfKind | 2,809 | 4,192 | 13 |
| HealAllEntities | 3,121 | 0 | 0 |

A zone:

| Method | ns/op | B/op | allocs/op |
|---|---|---|---|
| ZoneLayerSlots | 10.9 | 0 | 0 |
| ExitEntityByID | 13.8 | 0 | 0 |
| AddZoneLayer and RemoveZoneLayer | 33.5 | 0 | 0 |
| SendMessage | 43.0 | 48 | 2 |
| AddExit | 181 | 247 | 3 |

A world:

| Method | ns/op | B/op | allocs/op |
|---|---|---|---|
| AddZoneLayer and RemoveZoneLayer | 24.3 | 0 | 0 |
| ItemsHeld | 24.7 | 0 | 0 |
| SendPrivateMessage | 54.6 | 64 | 2 |
| InventorySlots | 65.1 | 160 | 1 |
| AddItemToCharacter | 124 | 192 | 3 |

## Reading

- **A read by an ID is 7 to 55 ns.** `CharacterByID` is the dearest, since it reads two tables (the character, then the body that holds its health, its maximum and its radius) and gives back a value that holds the cooldowns.
- **`HealAllEntities` walks every entity:** 3.1 µs for 300 entities (150 characters and 150 items), about 10 ns for each, when every character is at full health, so none is changed; each that is healed adds a copy and an event.
- **`EntityIDsOfKind` gives a list that it grows as it walks:** 2.8 µs, 13 allocations and 4.2 KB for the 150 items. The owner of the action that calls it can size the list first.
- **Adding an entity is 300 to 540 ns with its removal:** the table, the index of the kinds and the event that tells the clients.
