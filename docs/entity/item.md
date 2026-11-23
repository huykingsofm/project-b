# Item on the ground

`zonelayer/entity.ItemEntity`. Implements `state.Entity`. Held in the state of a zonelayer. A stack of one kind of item; it leaves the ground when picked up or when its expiry ends.

## Data and methods

The fields are private; the item is a value, made with `NewItemEntity(id, pos, itemID, count, expiry)`.

| Method | Returns | Meaning |
|---|---|---|
| `ID()` | `EntityID` | made of the ID of the zonelayer and the number it counted |
| `KindID()` | `KindID` | `ItemKindID` |
| `Position()` | `Vec` | position, in units |
| `Radius()` | `int32` | radius, in units |
| `BoundingBox()` | `Rect` | the square of side `2*Radius` around `Position` |
| `ItemID()` | `ItemID` | the kind of item |
| `Count()` | `uint32` | how many lie together |
| `Expiry()` | `uint64` | ticks it lies on the ground before the removal; 0 never; starts again each time it is put on the ground |
| `MovedTo(pos)` | `Entity` | the same stack at another position |

- Hash: common part, then item, count, expiry.
- Written as the number the zonelayer counted (not the whole ID), the position, the radius, the item, the count and the expiry.

## State methods (zonelayer)

| Method | Effect | Events |
|---|---|---|
| `AddItemEntity(itemID, count, at, expiry)` | puts a stack on the ground with a new entity ID; returns the ID, and false when the state has none to give | `ItemAppeared` |
| `RemoveItemEntity(id)` | takes the stack off the ground | `ItemDisappeared` |
| `ItemEntityByID(id)` | the stack | |

## Life of an item

1. On the ground: [`ZoneLayerItemEntityAddAction`](../action/zonelayer.md#zonelayeritementityaddaction), which calls `AddItemEntity` and sets the removal; every place that puts an item on the ground goes through it.
2. Picked up: [`ZoneLayerCharacterItemPickupAction`](../action/zonelayer.md#zonelayercharacteritempickupaction) removes it, then [`WorldCharacterItemAddAction`](../action/world.md#worldcharacteritemaddaction) puts it in the inventory.
3. Back on the ground (inventory full, or the message was not delivered): [`ZoneLayerItemEntityAddAction`](../action/zonelayer.md#zonelayeritementityaddaction) with the same kind, count, position and expiry, a new entity ID, and the expiry starts again.
4. Expiry ends: [`ZoneLayerItemEntityRemoveAction`](../action/zonelayer.md#zonelayeritementityremoveaction) takes it off the ground if it is still there.

## Inventory (world)

Not an entity: held by the world state (`state/world`), per character.

| Config | Meaning |
|---|---|
| `world.inventory.slots` | slots per character |
| `world.inventory.stack_limit` | items of one kind in a slot |

- A slot is a stack of one kind.
- `AddItemToCharacter` fills the slots that hold the kind first, then empty slots in order; returns what did not fit.
