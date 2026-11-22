# Actions

An action is one step of a scope's plan. Each belongs to exactly one scope: [world](world.md), [zone](zone.md) or [zonelayer](zonelayer.md). Code: [`sim/action`](../../server/internal/services/game/sim/action). Entities: [docs/entity](../entity/entity.md). Rules: [R25](../../plan/rule/r25-action-naming-and-files.md) (names).

## Naming

| Thing | Form |
|---|---|
| Type | `<Scope><Words>Action`, words from general to specific |
| File | `<scope>_<words>_action.go` |
| ID | `<Type>ID`, a constant of type `ID` in the register `id.go` |
| Cooldown key | `<Type>CooldownKey`, a constant of type `CooldownKey` in the register `cooldown_key.go` |

## Keys of an entry

Each action lists these keys, in this order, and leaves out the ones that do not apply.

| Key | Meaning |
|---|---|
| Kind | intent (from a command), message (from another engine), system (heal over time), made (added by another action); cancellable, standing |
| Priority | when it runs in the tick |
| Command | the command that makes it |
| Fields | what it carries |
| Reads | what it needs from elsewhere |
| Config | the config variables it uses |
| Cooldown | the key it sets |
| When busy | what it does while the cooldown is ahead |
| Schedule | when a system action runs |
| Standing | how it stays in the plan |
| Sent by | the actions that send it, and what a failed delivery means |
| Created by | the action that adds it to the plan |
| On failure | what a failed delivery of a message it sends means |
| On cancel | what it settles when cancelled |
| Steps | numbered, in order |
| Example | numbers and ticks |

## Priorities

A smaller number runs first; equal priorities run in the order they arrived.

| Name | Value |
|---|---|
| `PrioritySystem` | -200 |
| `PriorityIntent` | -100 |
| `PriorityModifier` | -50 |
| `PriorityResolve` | 0 |

## Registry

What must happen outside a scope goes in its registry and happens after the plan.

| Call | Target |
|---|---|
| `ToZoneLayer`, `ToZone`, `ToWorld` | a message to that scope |
| `ToSelf` | this action again, next tick |
| `ToSelfAt` | this action again, at a tick |
| `CancelWaiting` | cancels the actions a key names (an ID and, if given, an actor) that wait in the scheduler or were set for the next tick before this call; it takes effect when the tick is over |

## Failed delivery

Every message carries an `OnFail`: data, not a function, so the message can be written whole.

| Type (package `onfail`) | ID | What it does | Holds |
|---|---|---|---|
| `onfail.Retry{}` | 1 | sends the message again to the same address | nothing |
| `onfail.Redirect{To}` | 2 | sends it again to `To`, an engine of the same scope | an address |
| `onfail.Compensate{With}` | 3 | sends `With`, the message that undoes what the first set going | a message: where it goes, its actions, its own `OnFail` |
| `onfail.Notify{Key, Args}` | 4 | tells the actor of the message | a text key, arguments (strings) |
| `onfail.Escalate{To}` | 5 | gives the message to `To` to decide | an address |
| `onfail.Drop{}` | 6 | forgets the message and counts it | nothing |

| Package | Holds |
|---|---|
| `sim/action/message` | `Address`, `Scope`, `Header`, `Reason`, and the interfaces `Message`, `OnFail` and `Bouncer` |
| `sim/action/onfail` | the kinds above, each in a file of its own (`retry.go`, `redirect.go`, `compensate.go`, `notify.go`, `escalate.go`, `drop.go`), the register of their IDs, and their codec (`onfail.Encode`, `onfail.Decode`) |
| `sim/action` | the actions, and the messages that carry them (`MessageToZoneLayer`, `MessageToZone`, `MessageToWorld`) |

- `message.OnFail` is an interface: `ID()`, `Handle(Bouncer, Message, Reason)`, `Encode(w)`. A kind runs itself on the engine that sent the message through `message.Bouncer`.
- `onfail.Retry` and `onfail.Redirect` drop the message when it was tried `bounce.max_attempts` times, and every kind that sends again drops it while the engine stops.
- A message with no `OnFail` is dropped, and logged.

## Reading other scopes

- No scope keeps a copy of another's data; an action reads the latest state that scope published (`r.Stable`), which may be a few ticks old or missing.
- What an action does when it is missing is stated in its own entry.

| Read | From |
|---|---|
| map | `r.Stable.Zone(zoneID).TileMap()` |
| exits | `r.Stable.Zone(zoneID).ExitEntityByID` |
| zonelayers and capacity of a zone | `r.Stable.Zone(zoneID).ZoneLayerSlots()` |
| characters held by a zonelayer | `r.Stable.ZoneLayer(id).CharacterCount()` |

## Cooldowns

- A character has a cooldown per key: the tick at which it is free of what the key stands for.
- A key is defined in the file of its action, so any action can read another's.

## Index

| Scope | Action | Kind |
|---|---|---|
| world | [WorldChatPrivateSendAction](world.md#worldchatprivatesendaction) | intent |
| world | [WorldCharacterItemAddAction](world.md#worldcharacteritemaddaction) | message |
| zone | [ZoneCharacterEnterAction](zone.md#zonecharacterenteraction) | intent |
| zone | [ZoneChatSendAction](zone.md#zonechatsendaction) | intent |
| zonelayer | [ZoneLayerCharacterMoveAction](zonelayer.md#zonelayercharactermoveaction) | intent, cancellable |
| zonelayer | [ZoneLayerCharacterNormalAttackTriggerAction](zonelayer.md#zonelayercharacternormalattackaction) | intent |
| zonelayer | [ZoneLayerCharacterDamageAction](zonelayer.md#zonelayercharacterdamageaction) | made |
| zonelayer | [ZoneLayerCharacterShieldGrantAction](zonelayer.md#zonelayercharactershieldaction) | intent, standing |
| zonelayer | [ZoneLayerCharacterRegenAction](zonelayer.md#zonelayercharacterregenaction) | system |
| zonelayer | [ZoneLayerCharacterEnterAction](zonelayer.md#zonelayercharacterenteraction) | message |
| zonelayer | [ZoneLayerCharacterExitToAction](zonelayer.md#zonelayercharacterexittoaction) | intent |
| zonelayer | [ZoneLayerCharacterItemPickupAction](zonelayer.md#zonelayercharacteritempickupaction) | intent |
| zonelayer | [ZoneLayerItemEntityAddAction](zonelayer.md#zonelayeritementityaddaction) | message |
| zonelayer | [ZoneLayerItemEntityRemoveAction](zonelayer.md#zonelayeritementityremoveaction) | made |
| zonelayer | [ZoneLayerChatSendAction](zonelayer.md#zonelayerchatsendaction) | intent, message |
