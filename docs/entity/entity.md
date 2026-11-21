# Entities

What stands in a zonelayer. Code: [`sim/state/entity`](../../server/internal/services/game/sim/state/entity). The actions that change them are in [docs/action](../action/index.md).

## Kinds

| Kind | `KindID` | Document | Held by |
|---|---|---|---|
| Character body | 1 | [character.md](character.md) | zonelayer |
| Item on the ground | 2 | [item.md](item.md) | zonelayer |
| Exit | 3 | [exit.md](exit.md) | zone (stands in every zonelayer of it) |

- A `KindID` is added at the end of the list; a number is never changed or reused. 0 is no kind.

## `Entity` interface

| Method | Returns |
|---|---|
| `ID()` | `state.EntityID`, unique in the scope that holds the entity (one counter per scope) |
| `KindID()` | the kind |
| `Position()` | where it stands, in units |
| `Radius()` | how far it reaches from its position, in units; 0 is a point |
| `BoundingBox()` | smallest `Rect` that holds it; a grid files it by this |
| `MovedTo(pos)` | the entity at another position; the receiver does not change |
| `Hash(h)` | adds the entity to the state hash |

## `DamageableEntity` interface

An `Entity` with health. The state damages and heals through it and never asks the kind.

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
- **Add a kind**: a type that implements `Entity`, a `KindID` at the end of the list, a document here.
