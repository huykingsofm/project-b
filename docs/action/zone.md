# Zone actions

State: [`state/zone`](../../server/internal/services/game/sim/state/zone): the map, the exit entities, the zonelayers and their capacity, messages said in the zone. The zone keeps no count of who is in its zonelayers. Registry: `ZoneRegistry`. Conventions, keys and priorities: [index](index.md).

Crossing to another zone is not a zone action: see [`ZoneLayerCharacterExitToAction`](zonelayer.md#zonelayercharacterexittoaction).

## ZoneCharacterEnterAction

- Kind: intent
- Priority: `PriorityIntent`
- Command: `EnterWorldZoneCommand{Character}` (the whole character, made or loaded outside the core)
- Fields: `Character`
- Reads: published state of each zonelayer of the zone, for its count (a zonelayer that published nothing counts as empty); not waited for
- Steps:
  1. Picks the first zonelayer, in the zone's order, that holds fewer characters than its capacity; nothing if none.
  2. Sends [`ZoneLayerCharacterEnterAction`](zonelayer.md#zonelayercharacterenteraction) with an arrival position that is not a tile (so the map's spawn is used); dropped if not delivered.
- Example: two zonelayers of capacity 2; the first two characters go to zonelayer 1; once it has published two, the third goes to zonelayer 2.

## ZoneChatSendAction

- Kind: intent
- Priority: `PriorityResolve`
- Command: `ChatZoneCommand{FromID, Text}`
- Fields: `FromID`, `Text`
- Steps:
  1. `SendMessage`: last 100 kept; the events get it.
  2. For each zonelayer, sends [`ZoneLayerChatSendAction`](zonelayer.md#zonelayerchatsendaction) with the same character and text; dropped if not delivered.
- Example: tick 5 "hello zone"; recorded at tick 5; zonelayers 1 and 2 have it at tick 6.
