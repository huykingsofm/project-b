# Entities

What stands in a zone or a zonelayer. The interface `Entity` and the list of kinds are common and are in [`state`](../../server/game/sim/state/entity.go); each state holds the entities of the kinds it chooses, and they are in a package `entity` below the state, like its `event` package: [`zonelayer/entity`](../../server/game/sim/state/zonelayer/entity) (`CharacterEntity`, `ItemEntity`) and [`zone/entity`](../../server/game/sim/state/zone/entity) (`ExitEntity`). A state may hold a kind that another state holds too, and manage it in its own way. The actions that change them are in [docs/action](../action/index.md).

## Kinds

| Kind | `KindID` | Document | Held by |
|---|---|---|---|
| Character body | 1 | [character.md](character.md) | zonelayer |
| Item on the ground | 2 | [item.md](item.md) | zonelayer |
| Exit | 3 | [exit.md](exit.md) | zone (stands in every zonelayer of it) |

- A `KindID` is added at the end of the list in `state/entity.go`; a number is never changed or reused. 0 is no kind. The list is of the whole game, so a kind means one thing whichever state holds it.
- Each package `entity` has the codec of its own kinds (`entity.Encode`, `entity.Decode`), with a table of decoders by `KindID` and a test that fails when a kind it holds has none.
- A state keeps its entities in a `table.Table` by `EntityID`; the zonelayer has a column of the `KindID`, so `EntityIDsOfKind` finds the entities of a kind without walking all of them.

## `Entity` interface

| Method | Returns |
|---|---|
| `ID()` | `state.EntityID`, a number that the state that holds the entity counted, unique among the entities of that state ([IDs](../ids.md)) |
| `KindID()` | the kind |
| `Position()` | where it stands, in units |
| `Radius()` | how far it reaches from its position, in units; 0 is a point |
| `BoundingBox()` | smallest `Rect` that holds it; a grid files it by this |
| `MovedTo(pos)` | the entity at another position; the receiver does not change |
| `Hash(h)` | adds the entity to the state hash |

## `DamageableEntity` interface

In `state`, as `Entity` is. An `Entity` with health. The state damages and heals through it and never asks the kind.

| Method | Returns |
|---|---|
| `Health()` | current health |
| `MaxHealth()` | most health |
| `OnDamage(amount)` | the entity with health lowered, never below 0 |
| `OnHeal(amount)` | the entity with health raised, never above `MaxHealth` |

Implemented by: character body.

## Rules for every entity

- **Value type** (R29): a change builds a new value and replaces the old one in its container.
- **Units**: positions, radii and distances are in units; an entity may stand anywhere on the map. Only the map is cut into tiles (1024 units each) and only its constructor takes tiles.
- **Hash**: the ID, the kind and the bounding box, then the fields of the kind.
- **Add a kind**: a type that implements `state.Entity` in the package `entity` of the state that holds it, with a constructor (its fields are private), a `KindID` at the end of the list, a decoder in the table of that package, a document here.
