# Zonelayer actions

State: [`state/zonelayer`](../../server/game/sim/state/zonelayer): the entities of one copy of a zone's map ([character](../entity/character.md), [item](../entity/item.md); the [exits](../entity/exit.md) are the zone's), cooldowns, chat. Registry: `sim.Registry[*zonelayerstate.State]`. Conventions, keys and priorities: [index](index.md).

## Character

### ZoneLayerCharacterMoveAction

- Kind: intent, cancellable
- Priority: `PriorityIntent`
- Command: `ZoneLayerCharacterMoveCommand{CharacterID, Dir, Ticks}`
- Fields: `CharacterID`, `Dir`, `Ticks` (ticks left to move); private: whether the run has taken its place in the cooldown
- Reads: map, from the published zone; if missing, sets itself for the next tick
- Cooldown: `ZoneLayerCharacterMoveActionCooldownKey`, the first tick at which the character is not moving
- When busy: takes its place in the cooldown at once and sets itself for the tick the character is free
- Steps:
  1. Nothing if the character is not in the zonelayer, has no health or no speed, or `Ticks` is 0.
  2. First run only: `start` = later of this tick and the cooldown; cooldown = `start + Ticks`; if `start` is later than now, `ToSelfAt(start)` and stop.
  3. Reads the map.
  4. `MoveEntity` by `Character.Speed` units towards `Dir` (`TileMap.Reach` follows the whole path and stops at the last unit before the body, a square of the entity's radius, touches a tile that cannot be stood on; a diagonal across a corner needs both tiles beside it free). Cost: [benchmarks](../benchmark/move.md).
  5. `Ticks` -= 1; if any left, `ToSelf`.
- On cancel: `ApplyZoneLayerCancel`: if the run had taken its place and the cooldown is still ahead, the cooldown becomes this tick.
- Example: tick 10, `{East, 3}` then `{South, 2}`. Run 1: cooldown 13, moves at 10, 11, 12. Run 2: start 13, cooldown 15, moves at 13, 14. Ends three steps east and two south; the runs never overlap.

### ZoneLayerCharacterNormalAttackTriggerAction

- Kind: intent
- Priority: `PriorityIntent`
- Command: `ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID, TargetID}`
- Fields: `CharacterID`, `TargetID` (a character of the zonelayer)
- Config: `zonelayer.normal_attack.cooldown_ticks`
- Cooldown: `ZoneLayerCharacterNormalAttackTriggerActionCooldownKey`, set to this tick + the config
- When busy: does nothing
- Steps:
  1. Nothing if the character is not in the zonelayer or has no health.
  2. Nothing if the target is not in the zonelayer or has no health.
  3. Nothing if the cooldown is ahead.
  4. Sets the cooldown.
  5. Adds a [`ZoneLayerCharacterDamageAction`](#zonelayercharacterdamageaction) against the target's entity (amount 1, depth 1) behind itself.
- Example: tick 10 hits: cooldown 16, damage 1 lands in the same tick. Tick 12: nothing. Tick 16: hits again. A target not in the zonelayer: nothing, no cooldown.

### ZoneLayerCharacterDamageAction

- Kind: made
- Priority: `PriorityResolve`
- Fields: `SourceID`, `VictimID` (entities), `Amount`
- Created by: [`ZoneLayerCharacterNormalAttackTriggerAction`](#zonelayercharacternormalattackaction); found in the plan by ID only (`FindByID` with `ZoneLayerCharacterDamageActionID`)
- Steps:
  1. Nothing if `Amount` <= 0.
  2. `DamageEntity` on the victim (a [`DamageableEntity`](../entity/entity.md#damageableentity-interface)).
- Example: damage 1 on 100 health leaves 99; if a shield cancelled it earlier in the tick, it never runs.

### ZoneLayerCharacterShieldGrantAction

- Kind: intent, standing
- Priority: `PriorityModifier` (runs before the damage)
- Command: `ZoneLayerCharacterShieldGrantCommand{CharacterID, Duration}`
- Fields: `CharacterID`, `Duration` (ticks); private: start, end
- Standing: `ToSelf` every tick until its time is up or it blocked a damage; not stored in the state
- Steps:
  1. First run: end = this tick + `Duration`.
  2. Stops when time is up.
  3. `Find` damage actions in the plan by ID; cancels the first aimed at its character's entity and stops.
  4. Otherwise `ToSelf`.
- Example: shield for 100 ticks at tick 20; at tick 24 a damage is added; the shield runs first (-50 vs 0), cancels it and ends. Health unchanged; the next damage lands.

### ZoneLayerCharacterRegenAction

- Kind: system
- Priority: `PrioritySystem`
- Fields: `Every`, the ticks between two heals; 0 heals once
- Schedule: it sets itself again at `tick + Every` after each run (`ToSelfAt`); the first is set by `game.EnsureRegen(engine, RegenEvery)` when the cluster is built, 0 never
- Steps:
  1. `HealAllEntities(1)`: every damageable entity above 0 health gets 1, up to its most; one health-changed event per change.
  2. When `Every` is more than 0, `ToSelfAt(tick + Every, itself)`.
- Example: `RegenEvery: 10`, health 90 of 100: 91 at tick 10, 92 at tick 20.

## Enter and exit

### ZoneLayerCharacterEnterAction

- Kind: message
- Priority: `PriorityResolve`
- Fields: `Character` (health and cooldowns included), `At` (position asked for)
- Reads: map, from the published zone; if missing, `ToSelf` and stop
- Sent by: [`ZoneCharacterEnterAction`](zone.md#zonecharacterenteraction); [`ZoneLayerCharacterExitToAction`](#zonelayercharacterexittoaction), and the same action sent back to its own zonelayer as the compensation
- Steps:
  1. Nothing if the character is already in the zonelayer.
  2. Reads the map.
  3. If the body does not fit at `At` (`TileMap.FreeForBody`), uses the map's spawn.
  4. `AddCharacterEntity`.
- Note: a body that does not fit at the spawn either is put there anyway, and cannot move (`Reach` does not move a body that does not fit where it is); the map leaves room at its spawn for the bodies that enter.
- Example: a character comes through the east exit of zone 1 asking for tile (1,1) in zone 2; the place is free, so it stands there with its health and cooldowns.

### ZoneLayerCharacterExitToAction

- Kind: intent
- Priority: `PriorityIntent`
- Command: `ZoneLayerCharacterExitToCommand{CharacterID, ExitEntityID}`
- Fields: `CharacterID`, `ExitEntityID` (exit entity of the zone)
- Reads (published, may be a few ticks old):
  - the zone: the exit entity by ID
  - the zone the exit leads to: its zonelayers and capacity
  - each of those zonelayers: how many characters it holds
- On failure: `onfail.Compensate`: a [`ZoneLayerCharacterEnterAction`](#zonelayercharacterenteraction) with the same character comes back to this zonelayer at the position it left
- Steps:
  1. Nothing if the character is not in the zonelayer or has no health.
  2. Nothing if the zone is not published, the exit entity does not exist, or the character is not inside its `Area`.
  3. Nothing if the target zone is not published.
  4. Picks the first zonelayer of the target zone, in its order, with fewer characters than its capacity (unpublished counts as empty); nothing if none.
  5. `RemoveCharacterEntity`.
  6. Sends [`ZoneLayerCharacterEnterAction`](#zonelayercharacterenteraction) to the picked zonelayer, asking for the exit's `Arrival`.
- Note: two characters at the same tick may both find room for one; a zonelayer may hold one more than its capacity for a moment.
- Example: east exit of zone 1; zone 2 has one zonelayer of capacity 1, empty: the character leaves and appears at the arrival. If it already holds one, nothing happens. If the zonelayer was removed after it published, the message comes back and the character stays where it was.

## Item

### ZoneLayerCharacterItemPickupAction

- Kind: intent
- Priority: `PriorityIntent`
- Command: `ZoneLayerCharacterItemPickupCommand{CharacterID, ItemEntityID}`
- Fields: `CharacterID`, `ItemEntityID`
- Config: `zonelayer.pickup.radius` (units)
- On failure: `onfail.Compensate`: the stack returns to the ground as it was (same kind, count, place and expiry; new entity ID; the expiry starts again) through [`ZoneLayerItemEntityAddAction`](#zonelayeritementityaddaction)
- Steps:
  1. Nothing if the character is not in the zonelayer or has no health.
  2. Nothing if no item entity has that ID.
  3. Nothing if the `BoundingBox` of the character's body does not touch the item's `BoundingBox` grown by the radius on every side (a square).
  4. `RemoveItemEntity`: the whole stack leaves the ground.
  5. Sends [`WorldCharacterItemAddAction`](world.md#worldcharacteritemaddaction) with kind, count, place, expiry and this zonelayer.
- Example: a stack of 3 of kind 7 at (2500, 2500), character 1 at (1500, 2500) picks it up at tick 10: gone from the ground that tick; the world adds 3 to the inventory. Character 3 going for it the same tick finds it gone.

### ZoneLayerItemEntityAddAction

- Kind: message
- Priority: `PriorityResolve`
- Fields: `ItemID`, `Count`, `At`, `Expiry` (0: the item is never removed)
- Sent by: [`WorldCharacterItemAddAction`](world.md#worldcharacteritemaddaction) (what did not fit); [`ZoneLayerCharacterItemPickupAction`](#zonelayercharacteritempickupaction) (compensation); dropped if not delivered
- Steps:
  1. Nothing if `Count` is 0.
  2. `AddItemEntity`.
  3. If `Expiry` is above 0, `ToSelfAt(tick + Expiry)` a [`ZoneLayerItemEntityRemoveAction`](#zonelayeritementityremoveaction) for the new entity.
- Example: `Expiry` 30; a stack added at tick 10 is removed at tick 40. Room for 2 of a stack of 5 picked up at tick 10: the world returns 3; they are on the ground at tick 12 and removed at tick 42.

### ZoneLayerItemEntityRemoveAction

- Kind: made
- Priority: `PriorityResolve`
- Fields: `EntityID`
- Created by: [`ZoneLayerItemEntityAddAction`](#zonelayeritementityaddaction), set for the end of the item's expiry
- Steps:
  1. `RemoveItemEntity`; nothing if the item is no longer in the state (picked up; an entity ID is never used twice in a zonelayer).
- Example: an item picked up at tick 20 has no removal effect at tick 40; the stack that came back has a new entity and its own removal.

## Chat

### ZoneLayerChatSendAction

- Kind: intent, message
- Priority: `PriorityResolve`
- Command: `ZoneLayerChatSendCommand{FromID, Text}`
- Fields: `FromID`, `Text`
- Sent by: [`ZoneChatSendAction`](zone.md#zonechatsendaction), once per zonelayer; dropped if not delivered
- Steps:
  1. `SendMessage`: kept in the chat table (last 100); the events get it with its tick.
- Note: the sender need not be in this zonelayer (a zone message may come from another).
- Example: tick 5 "hello zonelayer": recorded, its clients told. A zone message at tick 5 reaches zonelayers 1 and 2 at tick 6.
