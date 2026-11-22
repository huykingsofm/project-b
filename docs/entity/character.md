# Character body

`zonelayer/entity.CharacterEntity`. Implements `state.Entity` and `state.DamageableEntity`. Held in the state of a zonelayer; which character it belongs to is kept there, not in the entity.

## Fields

| Field | Type | Meaning |
|---|---|---|
| `EntityID` | `EntityID` | ID in the zonelayer |
| `Pos` | `Vec` | position, in units |
| `Rad` | `int32` | radius, in units; the body is a square that reaches `Rad` units from `Pos` on every side, of any size (a boss may cover many tiles) |
| `Hp` | `int32` | health |
| `MaxHp` | `int32` | most health |

## Behavior

- `BoundingBox`: the square of side `2*Rad` around `Pos`.
- `OnDamage`, `OnHeal`: health clamped to `[0, MaxHp]`.
- `MovedTo`: the same body at another position.
- Hash: common part, then `Hp`, `MaxHp`.

## Character (the data beside the body)

`zonelayer.Character`, one per character in the zonelayer, found by `CharacterID`.

| Field | Meaning |
|---|---|
| `CharacterID` | the character |
| `Name` | name |
| `Speed` | units moved in one tick |
| `Radius` | radius of the body, in units; not negative (`NewCharacter` returns an error otherwise) |
| `Cooldowns` | tick at which the character is free, per key (see [action index](../action/index.md#cooldowns)) |

`Health`, `MaxHealth` and `Radius` of a `Character` read are those of the body.

## State methods (zonelayer)

| Method | Effect | Events |
|---|---|---|
| `AddCharacterEntity(character, at)` | gives the character a new body at `at`; returns its ID | `CharacterEntered` |
| `RemoveCharacterEntity(id)` | takes the character and its body out, whole | `CharacterLeft` |
| `CharacterEntityByCharacterID(characterID)` | the body of a character | |
| `CharacterByID(characterID)` | the character, health taken from the body | |
| `MoveEntity(tiles, id, dir, distance)` | moves any entity by `distance` units towards `dir`, as far as its body (its radius) can go without touching a tile that cannot be stood on (`TileMap.Reach`) | `EntityMoved` |
| `DamageEntity(id, amount)` | `OnDamage` on a damageable entity | `EntityHealthChanged`, `EntityDied` at 0 |
| `HealAllEntities(amount)` | `OnHeal` on every damageable entity above 0 health | `EntityHealthChanged` for each that changed |
