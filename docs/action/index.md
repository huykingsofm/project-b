# Actions

An action is one step of the plan of an engine. Each belongs to exactly one scope of this game (the state it changes) and is a `sim.Action[S]` for the state type `S` of that scope: [world](world.md), [zone](zone.md) or [zonelayer](zonelayer.md). Code: [`server/game/sim/action`](../../server/game/sim/action); what an action is made of is in [`platform/sim`](../../platform/sim). Entities: [docs/entity](../entity/entity.md). Rules: [R25](../../plan/rule/r25-action-naming-and-files.md) (names).

## Naming

| Thing | Form |
|---|---|
| Type | `<Scope><Words>Action`, words from general to specific |
| File | `<scope>_<words>_action.go` |
| ID | `<Type>ID`, a constant of type `sim.ActionID` in the register `id.go` |
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

What must happen outside a state goes in its `sim.Registry[S]` and happens after the plan.

| Call | Target |
|---|---|
| `Send[T](to, onFail, actions...)` | a message to the state with the ID `to`, whose actions are `sim.Action[T]` |
| `Get[T](id)` | the latest state that the engine with the ID published, as a `T` (see [Reading other states](#reading-other-states)) |
| `ToSelf` | this action again, next tick |
| `ToSelfAt` | this action again, at a tick (a tick that is not in the future is the next one) |

## Failed delivery

Every message carries an `OnFail`: data, not a function, so the message can be written whole.

| Type (package `onfail`) | ID | What it does | Holds |
|---|---|---|---|
| `onfail.Retry{}` | 1 | sends the message again to the same address | nothing |
| `onfail.Redirect{To}` | 2 | sends it again to `To`, another engine | a state ID |
| `onfail.Compensate{With}` | 3 | sends `With`, the message that undoes what the first set going | a message: where it goes, its actions, its own `OnFail` |
| `onfail.Notify{Key, Args}` | 4 | tells the actor of the message | a text key, arguments (strings) |
| `onfail.Escalate{To}` | 5 | gives the message to `To` to decide | a state ID |
| `onfail.Drop{}` | 6 | forgets the message and counts it | nothing |

| Package | Holds |
|---|---|
| `platform/sim` | `Header`, `DeliveryFailure`, the interfaces `Message`, `OnFail` and `Bouncer`, the message that carries actions (`ActionMessage[S]`, made with `NewActionMessage` or `Registry.Send`) and the codecs |
| `platform/sim/onfail` | the kinds above, each in a file of its own (`retry.go`, `redirect.go`, `compensate.go`, `notify.go`, `escalate.go`, `drop.go`), the register of their IDs, and `onfail.Register(codecs)` |
| `server/game/sim/action` | the actions, and `action.Register(codecs)` |

- `sim.OnFail` is an interface: `ID()`, `Handle(Bouncer, Message, DeliveryFailure)`, `Encode(w)`. A kind runs itself on the engine that sent the message through `sim.Bouncer`.
- `onfail.Retry`, `onfail.Redirect` and `onfail.Escalate` drop the message when it was tried `bounce.max_attempts` times, and every kind that sends again drops it while the engine stops.
- `onfail.Retry` also drops the message, at once, when the state it is for is gone (`DeliveryFailureRetired`): sending it again fails the same way, so its `OnFail` is asked once and not after every attempt. `Redirect` and `Escalate` send to another engine, which a state that is gone is the very case for, and `Compensate` and `Notify` send something else or tell an actor, so they act as always.

### Why a message could not be delivered

| `sim.DeliveryFailure` | Number | Meaning |
|---|---|---|
| `DeliveryFailureRetired` | 1 | the state was removed |
| `DeliveryFailureNotStarted` | 2 | the state exists, but its engine does not run yet; the message can be sent again |
| `DeliveryFailureMoved` | 3 | what the message is about has gone to another engine |
| `DeliveryFailureStopping` | 4 | the engine, or the cluster, is stopping |
| `DeliveryFailureFull` | 5 | the inbox of the engine is full |

- `DeliveryFailure.Gone()` is true for `DeliveryFailureRetired`: the state is not coming back.
- The router says `DeliveryFailureRetired` for an engine that was retired, and `DeliveryFailureNotStarted` for any other ID with no engine: it does not know which of those IDs are given.
- A message with no `OnFail` is dropped, and logged.

### What ends an action that repeats

| Question | Answer |
|---|---|
| Who ends an action that sets itself again? | the action: when its actor is no longer in the state it does nothing and does not set itself again |
| What stops an action in the plan of this tick from running? | `Plan.Cancel`, which takes effect at once |

## Reading other states

- No state keeps a copy of another's data; an action reads the latest state that the engine with an ID published, with `Registry.Get[T](id)`, which may be a few ticks old or missing.
- `T` is the Go type of the state or an interface of its read methods (`ReadOnlyZoneState`, `ReadOnlyZoneLayerState`), so an action cannot change a state that is not its own. It returns `sim.ErrStateNotFound` when nothing was published and `sim.ErrStateWrongType` when the state is not a `T`.
- A state keeps the IDs of the states it needs to read: a zonelayer its zone and its world, a zone its world (and its exits the zones they lead to), the world every zone and zonelayer. What an action does when a read fails is stated in its own entry.

| Read | From |
|---|---|
| map | `Get[zonestate.ReadOnlyZoneState](zoneID)`, then `TileMap()` |
| exits | the same, then `ExitEntityByID` |
| zonelayers and capacity of a zone | the same, then `ZoneLayerSlots()` |
| characters held by a zonelayer | `Get[zonelayerstate.ReadOnlyZoneLayerState](id)`, then `CharacterCount()` |
| every zone and zonelayer | `Get[worldstate.ReadOnlyWorldState](worldID)`, then `ZoneIDs()`, `ZoneLayerIDs()` |

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
