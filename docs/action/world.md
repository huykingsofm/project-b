# Actions of the world

The world is the scope above the zones. These are the actions it runs. They change the state in [`state/world`](../../server/internal/services/game/sim/state/world): the private messages between players. What must happen outside the world goes in its registry (`WorldRegistry`) and happens when the plan is done. The actions of the other scopes are in [zone.md](zone.md) and [zonelayer.md](zonelayer.md). The rules for naming and files are in [R25](../../plan/rule/r25-action-naming-and-files.md).

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

Priorities: `PrioritySystem` (-200) runs first, then `PriorityIntent` (-100), `PriorityModifier` (-50) and `PriorityResolve` (0). A smaller number runs first; equal priorities run in the order they arrived.

## Index

| Group | Action | Kind |
|---|---|---|
| chat | [WorldChatPrivateSendAction](#worldchatprivatesendaction) | intent |

---

## Chat

### WorldChatPrivateSendAction

**Where.** `WorldChatPrivateSendAction` in `world_chat_private_send_action.go`; ID `WorldChatPrivateSendActionID`; priority `PriorityResolve`; author: the player who sent it (`FromID`, a `PlayerID`, not a character).

**Purpose.** A player sends a private message to another player.

**Reason.** A private message is between players, not between characters: a character is not always played by a player, and a player may have no character in the world at that moment. So the world manages it alone, and no zone or zonelayer takes part.

**Context.** Runs in the world. Does not need the map, and does not need to know where either player is.

**Fields.** `FromID`, `ToID` (both `PlayerID`), `Text`.

**Command.** `ChatPrivateWorldCommand{FromID, ToID, Text}`.

**What it does.** Calls `SendPrivateMessage`: the message is kept in the world's table of private messages (the last 1000, found by who sent it and who it is for), and the events get the message, tick included, so that the clients of the two players can be told.

**Example.** At tick 5 player 10 sends "psst" to player 20. The world records it at tick 5 and tells the output; no zonelayer or zone sees it.
