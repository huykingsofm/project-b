# World actions

State: [`state/world`](../../server/internal/services/game/sim/state/world): private messages between players, inventories of characters. Registry: `WorldRegistry`. Conventions, keys and priorities: [index](index.md).

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
