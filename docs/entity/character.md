# Character body

`zonelayer/entity.CharacterEntity`. Implements `state.Entity` and `state.DamageableEntity`. Held in the state of a zonelayer; which character it belongs to is kept there, not in the entity.

## Data and methods

The fields are private; the body is a value, and a method that changes it returns a changed copy. It is made with `NewCharacterEntity(id, pos, radius, health, maxHealth)`.

| Method | Returns | Meaning |
|---|---|---|
| `ID()` | `EntityID` | made of the ID of the zonelayer and the number it counted |
| `KindID()` | `KindID` | `CharacterKindID` |
| `Position()` | `Vec` | position, in units |
| `Radius()` | `int32` | radius, in units; the body is a square that reaches `Radius` units from `Position` on every side, of any size (a boss may cover many tiles) |
| `BoundingBox()` | `Rect` | the square of side `2*Radius` around `Position` |
| `Health()`, `MaxHealth()` | `int32` | health, and most health |
| `OnDamage(amount)`, `OnHeal(amount)` | `DamageableEntity` | the same body with the health clamped to `[0, MaxHealth()]` |
| `MovedTo(pos)` | `Entity` | the same body at another position |

- Hash: common part, then health, most health.
- Written as the number the zonelayer counted (not the whole ID: the zonelayer that holds it is the owner), the position, the radius, the health and the most health.

## Character (the data beside the body)

`zonelayer.Character`, one per character in the zonelayer, found by `CharacterID`.

Its fields are private too:

| Method | Meaning |
|---|---|
| `CharacterID()` | the character |
| `Name()` | name |
| `Speed()` | units moved in one tick |
| `Radius()` | radius of the body, in units; not negative (`NewCharacter` returns an error otherwise) |
| `Health()`, `MaxHealth()` | health and most health |
| `CooldownUntil(key)`, `CooldownKeys()` | tick at which the character is free, per key (see [action index](../action/index.md#cooldowns)) |

`Health`, `MaxHealth` and `Radius` of a `Character` read from the state are those of the body.

## State methods (zonelayer)

| Method | Effect | Events |
|---|---|---|
| `AddCharacterEntity(character, at)` | gives the character a new body at `at`; returns its ID, and false when the state has none to give | `CharacterEntered` |
| `RemoveCharacterEntity(characterID)` | takes the character and its body out, whole | `CharacterLeft` |
| `CharacterEntityByCharacterID(characterID)` | the body of a character | |
| `CharacterByID(characterID)` | the character, health taken from the body | |
| `MoveEntity(tiles, id, dir, distance)` | moves any entity by `distance` units towards `dir`, as far as its body (its radius) can go without touching a tile that cannot be stood on (`TileMap.Reach`) | `EntityMoved` |
| `DamageEntity(id, amount)` | `OnDamage` on a damageable entity | `EntityHealthChanged`, `EntityDied` at 0 |
| `HealAllEntities(amount)` | `OnHeal` on every damageable entity above 0 health | `EntityHealthChanged` for each that changed |
