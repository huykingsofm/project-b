# Zone actions

State: [`state/zone`](../../server/game/sim/state/zone): the map, the exit entities, the zonelayers and their capacity, messages said in the zone. The zone keeps no count of who is in its zonelayers. Registry: `sim.Registry[*zonestate.State]`. Conventions, keys and priorities: [index](index.md).

Crossing to another zone is not a zone action: see [`ZoneLayerCharacterExitToAction`](zonelayer.md#zonelayercharacterexittoaction).

## ZoneCharacterEnterAction

- Kind: intent
- Priority: `PriorityIntent`
- Command: `ZoneCharacterEnterCommand{Character}` (the whole character, made or loaded outside the core)
- Fields: `Character`
- Reads: published state of each zonelayer of the zone, for its count (a zonelayer that published nothing counts as empty); not waited for
- Steps:
  1. Picks the first zonelayer, in the zone's order, that holds fewer characters than its capacity; nothing if none.
  2. Sends [`ZoneLayerCharacterEnterAction`](zonelayer.md#zonelayercharacterenteraction) with an arrival position that is not a tile (so the map's spawn is used); dropped if not delivered.
- Example: two zonelayers of capacity 2; the first two characters go to zonelayer 1; once it has published two, the third goes to zonelayer 2.

## ZoneChatSendAction

- Kind: intent
- Priority: `PriorityResolve`
- Command: `ZoneChatSendCommand{FromID, Text}`
- Fields: `FromID`, `Text`
- Steps:
  1. `SendMessage`: last 100 kept; the events get it.
  2. For each zonelayer, sends [`ZoneLayerChatSendAction`](zonelayer.md#zonelayerchatsendaction) with the same character and text; dropped if not delivered.
- Example: tick 5 "hello zone"; recorded at tick 5; zonelayers 1 and 2 have it at tick 6.

## ZoneZoneLayerAddAction

- Kind: message
- Priority: `PriorityResolve`
- Fields: `ZoneLayerID`, `Capacity`
- Sent by: [`ZoneLayerAnnounceToZoneAction`](zonelayer.md#zonelayerannouncetozoneaction); dropped if not delivered
- Steps:
  1. `AddZoneLayer`: the zone knows the zonelayer, with room for `Capacity` characters.

## ZoneAnnounceToWorldAction

- Kind: system
- Priority: `PriorityResolve`
- Fields: none
- Set by: [`WorldZoneAddAction`](world.md#worldzoneaddaction), as an action that the zone starts with, for its first tick
- Steps:
  1. Sends [`WorldZoneRegisterAction`](world.md#worldzoneregisteraction) to the world of the zone with its own ID (`Context.Self`); retried if not delivered.
- Note: only the zone knows its own ID, which the coordinator gave it.

## ZoneExitAddAction

- Kind: message
- Priority: `PriorityResolve`
- Fields: `Area`, `ToZoneID`, `Arrival`
- Sent by: [`WorldZoneExitAddAction`](world.md#worldzoneexitaddaction); dropped if not delivered
- Steps:
  1. `AddExit`: an area that is empty or not in the map of the zone, or a target that is not a zone, adds nothing; otherwise the zone has an exit entity with the next number, and the events get `ExitAdded`.

