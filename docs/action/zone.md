# Actions of a zone

A zone is a map with the zonelayers that run it. These are the actions it runs. They change the state in [`state/zone`](../../server/internal/services/game/sim/state/zone): the map, the zonelayers it has and their capacity, and the messages said in the zone. The zone does not keep count of who is in its zonelayers: an action that needs a count reads it from what the zonelayers published. What must happen outside the zone goes in its registry (`ZoneRegistry`) and happens when the plan is done. The actions of the other scopes are in [zonelayer.md](zonelayer.md) and [world.md](world.md). The rules for naming and files are in [R25](../../plan/rule/r25-action-naming-and-files.md).

## How to read this file

Every action has these sections:

- **Where**: its type, its file, its ID type, its priority, and its author (`Actor()`) when it has one.
- **Purpose**: what it does, in one sentence.
- **Reason**: why it exists.
- **Context**: when it runs, and what it needs from elsewhere.
- **Fields**: what it carries.
- **What it does**: the steps, in order.
- **Example**: a concrete case, with ticks.

and, depending on its kind:

- **Command** (an *intent action*, made from what a client asked): the command that makes it.
- **On failure**: what a failed delivery of a message it sends means.

Priorities: `PrioritySystem` (-200) runs first, then `PriorityIntent` (-100), `PriorityModifier` (-50) and `PriorityResolve` (0). A smaller number runs first; equal priorities run in the order they arrived.

## Index

| Group | Action | Kind |
|---|---|---|
| character | [ZoneCharacterEnterAction](#zonecharacterenteraction) | intent |
| chat | [ZoneChatSendAction](#zonechatsendaction) | intent |

Crossing from one zone to another is not an action of a zone: the character does it from its zonelayer, in [`ZoneLayerCharacterExitToAction`](zonelayer.md#zonelayercharacterexittoaction), by reading what the zones published.

---

## Character

### ZoneCharacterEnterAction

**Where.** `ZoneCharacterEnterAction` in `zone_character_enter_action.go`; ID `ZoneCharacterEnterActionID`; priority `PriorityIntent`; author: the character (`Character.CharID`).

**Purpose.** Puts a character that entered the world in a zonelayer of the zone.

**Reason.** A character that enters the world is in no zonelayer yet, and only the zone knows its zonelayers and how many characters each takes. So the zone picks one and sends it the new character.

**Context.** Runs in the zone the client entered. Does not need the map. Counts from what the zonelayers published, which may be a few ticks old, and does not wait for the zonelayer to answer.

**Fields.** `Character` (the whole character, with the health and cooldowns it was saved with; it is made or loaded outside the core, which has no storage).

**Command.** `EnterWorldZoneCommand{Character}`.

**What it does.**
1. Takes the zonelayers of the zone in the order it has them, and picks the first that holds fewer characters than its capacity (a zonelayer that published nothing counts as empty). Does nothing if there is none.
2. Sends the character as it is to that zonelayer in a `ZoneLayerCharacterEnterAction`, with an arrival position that is not a tile, so the zonelayer uses the spawn point of the map. Dropped if it cannot be delivered.

**Example.** Zone 1 has two zonelayers of capacity 2. The first two characters that enter go to zonelayer 1; once zonelayer 1 has published that it holds two, the third goes to zonelayer 2.

---

## Chat

### ZoneChatSendAction

**Where.** `ZoneChatSendAction` in `zone_chat_send_action.go`; ID `ZoneChatSendActionID`; priority `PriorityResolve`; author: the character who said it (`FromID`).

**Purpose.** A message said in a zone is heard in every zonelayer of it.

**Reason.** The zone is the scope that knows all its zonelayers. For now it gives the message to each of them one by one; there is no separate action to fan it out.

**Context.** Does not need the map.

**Fields.** `FromID`, `Text`.

**Command.** `ChatZoneCommand{FromID, Text}`.

**On failure.** Each message to a zonelayer is dropped if it cannot be delivered.

**What it does.**
1. Records the message in the zone (`SendMessage`: the last 100 are kept, and the events get it).
2. For each zonelayer of the zone, sends it a `ZoneLayerChatSendAction` with the same character and text.

**Example.** At tick 5 character 1 says "hello zone" to zone 1. The zone records it at tick 5, and at tick 6 zonelayers 1 and 2 both have it.
