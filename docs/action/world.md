# World actions

State: [`state/world`](../../server/game/sim/state/world): private messages between players, inventories of characters. Registry: `sim.Registry[*worldstate.State]`. Conventions, keys and priorities: [index](index.md).

## WorldChatPrivateSendAction

- Kind: intent
- Priority: `PriorityResolve`
- Command: `WorldChatPrivateSendCommand{FromID, ToID, Text}`
- Fields: `FromID`, `ToID` (`PlayerID`), `Text`
- Steps:
  1. `SendPrivateMessage`: kept in the table of private messages (last 1000, found by sender and receiver); the events get the message with its tick.
- Example: tick 5, player 10 sends "psst" to player 20; the world records it at tick 5; no zone or zonelayer sees it.

## WorldCharacterItemAddAction

- Kind: message
- Priority: `PriorityResolve`
- Fields: `CharacterID`, `ItemID`, `Count`, `FromZoneLayerID`, `At`, `Expiry`
- Config: `world.inventory.slots`, `world.inventory.stack_limit` (see [item](../entity/item.md#inventory-world))
- Sent by: [`ZoneLayerCharacterItemPickupAction`](zonelayer.md#zonelayercharacteritempickupaction); dropped if not delivered
- Steps:
  1. `AddItemToCharacter`: fills the slots that hold the kind, then empty slots; the events get what was added.
  2. What does not fit goes to `FromZoneLayerID` in a [`ZoneLayerItemEntityAddAction`](zonelayer.md#zonelayeritementityaddaction) with the same place and expiry.
- Example: slots of 99, one holds 90 of kind 7; a stack of 20 arrives: that slot 99, an empty slot 11, nothing returns. With every slot full, all 20 return.

## WorldZoneAddAction

- Kind: intent
- Priority: `PriorityResolve`
- Command: `WorldZoneAddCommand{Tiles}`
- Fields: `Tiles` (the map; copied with `Clone`)
- Steps:
  1. `CreateEngine` with the tag `TagZone` and a zone state of this world (`Context.Self`) on the map. The world lists the zone when the cluster answers, not here (R28): `State.HandleDirective` takes the answer.
- Example: tick 1 the command; the cluster makes the zone and answers; at tick 2 the world lists it.

## WorldZoneLayerAddAction

- Kind: intent
- Priority: `PriorityResolve`
- Command: `WorldZoneLayerAddCommand{ZoneID, Capacity, RegenEvery}`
- Fields: `ZoneID`, `Capacity`, `RegenEvery` (0: never heals)
- Steps:
  1. `CreateEngine` with the tag `TagZoneLayer`, an empty zonelayer state of the zone and this world, and the actions it runs once at its first tick: [`ZoneLayerAnnounceToZoneAction`](zonelayer.md#zonelayerannouncetozoneaction) and, when `RegenEvery` is more than 0, [`ZoneLayerCharacterRegenAction`](zonelayer.md#zonelayercharacterregenaction), which heals at once and sets itself again every `RegenEvery` ticks. The world lists the zonelayer when the cluster answers.

## WorldZoneExitAddAction

- Kind: intent
- Priority: `PriorityResolve`
- Command: `WorldZoneExitAddCommand{FromZoneID, Area, ToZoneID, Arrival}`
- Fields: `FromZoneID`, `Area` (in units), `ToZoneID`, `Arrival` (in units, in `ToZoneID`)
- Steps:
  1. When the world lists both zones, sends [`ZoneExitAddAction`](zone.md#zoneexitaddaction) to `FromZoneID`; dropped if not delivered. Another zone is not joined: nothing happens.
- Note: one way. To join two zones both ways, two commands.

