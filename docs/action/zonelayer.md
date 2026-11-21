# Actions of a zonelayer

A zonelayer is one copy of a zone's map, with the characters in it. These are the actions it runs. They change the state in [`state/zonelayer`](../../server/internal/services/game/sim/state/zonelayer) and nothing else; what must happen outside the zonelayer goes in its registry (`ZoneLayerRegistry`) and happens when the plan is done. The actions of the other scopes are in [zone.md](zone.md) and [world.md](world.md). The rules for naming and files are in [R25](../../plan/rule/r25-action-naming-and-files.md).

## How to read this file

Every action has these sections:

- **Where**: its type, its file, its ID type, its priority, and its author (`Actor()`) when it has one.
- **Purpose**: what it does, in one sentence.
- **Reason**: why it exists.
- **Context**: when it runs, and what it needs from elsewhere.
- **Fields**: what it carries.
- **What it does**: the steps, in order.
- **Example**: a concrete case, with ticks.

and, depending on its kind, some of these:

- **Command** (an *intent action*, made from what a client asked): the command that makes it.
- **Cooldown** and **When busy** (an action that sets a cooldown): the key, and the action's own rule for a character that is still busy.
- **Sent by** and **On failure** (a *message action*, sent by another engine): who sends it, and what a failed delivery of a message it sends means.
- **Schedule** (a *system action*, run by a cron): when it runs.
- **How it stands** (a *standing action*): how it stays in the plan.
- **On cancel** (a *cancellable action*): what it settles when another action cancels it.
- **Created by** (an action made by another action): who adds it to the plan.

Priorities: `PrioritySystem` (-200) runs first, then `PriorityIntent` (-100), `PriorityModifier` (-50) and `PriorityResolve` (0). A smaller number runs first; equal priorities run in the order they arrived.

## Index

| Group | Action | Kind |
|---|---|---|
| character | [ZoneLayerCharacterMoveAction](#zonelayercharactermoveaction) | intent, cancellable |
| character | [ZoneLayerCharacterNormalAttackAction](#zonelayercharacternormalattackaction) | intent |
| character | [ZoneLayerCharacterDamageAction](#zonelayercharacterdamageaction) | made by another action |
| character | [ZoneLayerCharacterShieldAction](#zonelayercharactershieldaction) | intent, standing |
| character | [ZoneLayerCharacterRegenAction](#zonelayercharacterregenaction) | system (cron) |
| character enter and exit | [ZoneLayerCharacterEnterAction](#zonelayercharacterenteraction) | message |
| character enter and exit | [ZoneLayerCharacterExitToAction](#zonelayercharacterexittoaction) | intent |
| chat | [ZoneLayerChatSendAction](#zonelayerchatsendaction) | intent and message |

## What an action reads from other scopes

No scope keeps a copy of what another scope holds. An action that needs something of another scope reads it from the latest state that scope published (`r.Stable`), which may be a few ticks old, and which may be missing when that scope has not published yet. What an action does then is its own rule, given in its section.

- **The map** belongs to the zone. Actions that need it (move, enter, exit) read it from `r.Stable.Zone(s.ZoneID).Tiles`.
- **Counts** of the characters in a zonelayer are read from the published state of that zonelayer (`r.Stable.ZoneLayer(id).CharacterCount()`), by the action that needs the count.

---

## Character

### ZoneLayerCharacterMoveAction

**Where.** `ZoneLayerCharacterMoveAction` in `zonelayer_character_move_action.go`; ID `ZoneLayerCharacterMoveActionID`; priority `PriorityIntent`; author: the character (`CharID`).

**Purpose.** A character holds a direction for a number of ticks and moves one step in each of them.

**Reason.** A player's movement arrives as runs ("this way for N ticks"). The runs must be applied in the order they were sent and never two at once, and an effect must be able to see and change the movement of a character, so each tick of a run is an action in the plan.

**Context.** Runs in the zonelayer that holds the character. Needs the map, from the zone's published state.

**Fields.** `CharID`, `Dir`, `Ticks` (ticks still to move). One field is private: whether the run has taken its place in the cooldown yet.

**Command.** `MoveZoneLayerCommand{CharID, Dir, Ticks}`.

**Cooldown.** Key `ZoneLayerCharacterMoveActionCooldownKey`: the first tick at which the character is not moving any more. Another action may read it.

**When busy.** A run that finds the character still moving does not give up and does not push in: it takes its place in the cooldown at once and is set for the tick the character is free.

**What it does.**
1. Does nothing if the character is not in the zonelayer, has no health, or `Ticks` is 0.
2. The first time it runs: `start` is the later of this tick and the cooldown; the cooldown becomes `start + Ticks`; if `start` is later than this tick, the action sets itself for `start` (`ToSelfAt`) and stops.
3. Reads the zone's map. If the zone has not published yet, sets itself for the next tick (`ToSelf`) and stops.
4. Moves the entity one step (`zonelayer.move.step` tiles, default 1) with `MoveEntity`; a wall stops it.
5. Takes one from `Ticks`; if any are left, sets itself for the next tick.

**On cancel.** `ApplyZoneLayerCancel`: if the run had taken its place in the cooldown and the cooldown is still ahead, the cooldown is brought back to this tick, so the character is free.

**Example.** At tick 10 the client sends `{East, 3}` and then `{South, 2}`. The first run starts at 10, sets the cooldown to 13 and moves at ticks 10, 11, 12. The second finds the cooldown at 13, so its start is 13: it sets the cooldown to 15, is set for tick 13, and moves at ticks 13 and 14. The character ends three tiles east and two south, and the two runs never overlapped.

### ZoneLayerCharacterNormalAttackAction

**Where.** `ZoneLayerCharacterNormalAttackAction` in `zonelayer_character_normal_attack_action.go`; ID `ZoneLayerCharacterNormalAttackActionID`; priority `PriorityIntent`; author: the character (`CharID`).

**Purpose.** A character hits a target, another character of the zonelayer, with the normal attack.

**Reason.** An attack is an intent with a result of its own (the damage). Keeping the result as a separate action, the smallest unit of an attack, added to the plan behind the attack, lets effects see the hit before it lands. A skill will be another attack, with its own cooldown.

**Context.** Runs in the zonelayer that holds the character. Does not need the map.

**Fields.** `CharID`, `TargetID` (the character it hits).

**Command.** `NormalAttackZoneLayerCommand{CharID, TargetID}`.

**Cooldown.** Key `ZoneLayerCharacterNormalAttackActionCooldownKey`: the first tick at which the character can attack again. Set to this tick plus `zonelayer.normal_attack.cooldown_ticks` (default 6).

**When busy.** An attack that finds the character on cooldown does nothing.

**What it does.**
1. Does nothing if the character is not in the zonelayer or has no health.
2. Does nothing if the target is not in the zonelayer or has no health.
3. Does nothing if the cooldown is still ahead.
4. Sets the cooldown.
5. Adds a `ZoneLayerCharacterDamageAction` against the entity of the target (amount 1 for now, written in the code; later it will come from the attack of the attacker and the defence of the target; depth 1) to the plan behind itself.

**Example.** At tick 10 an attack hits: the cooldown becomes 16 and a damage of 1 is added to the plan in the same tick and lands in it. An attack at tick 12 finds the cooldown and does nothing; one at tick 16 hits again. An attack on a character that is not in the zonelayer does nothing and sets no cooldown.

### ZoneLayerCharacterDamageAction

**Where.** `ZoneLayerCharacterDamageAction` in `zonelayer_character_damage_action.go`; ID `ZoneLayerCharacterDamageActionID`; priority `PriorityResolve`; no author.

**Purpose.** Takes health from the character that has an entity as its body.

**Reason.** The place where health is lost. Because it is its own action in the plan, a standing action that runs before it can lower or cancel it.

**Context.** Runs in the zonelayer where the victim is. Does not need the map.

**Fields.** `SourceID`, `VictimID` (entities), `Amount`.

**Created by.** `ZoneLayerCharacterNormalAttackAction`, which adds it to the plan. It has no command and is never sent in a message. It has no author, so it is found in the plan by its ID only: `PlanKey{ID: ZoneLayerCharacterDamageActionID{}}`.

**What it does.**
1. Does nothing if its amount is 0 or less.
2. Calls `DamageEntity` on the victim: health goes down, never below 0, and the events get the change of health, and a death when it reaches 0.

**Example.** A damage of 1 against a character with 100 health leaves it with 99. If a shield cancelled it earlier in the same tick, it never runs.

### ZoneLayerCharacterShieldAction

**Where.** `ZoneLayerCharacterShieldAction` in `zonelayer_character_shield_action.go`; ID `ZoneLayerCharacterShieldActionID`; priority `PriorityModifier`; author: the character (`CharID`).

**Purpose.** Blocks the first damage that is aimed at a character.

**Reason.** The pattern for an effect that changes an action it was not written for. It shows how a standing action finds other actions in the plan through the plan's index and cancels them.

**Context.** Runs in the zonelayer that holds the character. It runs before the damage lands because its priority is smaller than the damage's.

**Fields.** `CharID`, `Duration` (ticks it stands, counted from the first tick it runs). Two fields are private: when it started and when it ends.

**Command.** `GrantShieldZoneLayerCommand{CharID, Duration}`.

**How it stands.** It stays in the plan by setting itself for the next tick (`ToSelf`) in every tick until its time is up or it has blocked a damage. It is not stored in the state.

**What it does.**
1. The first time it runs, sets its end to this tick plus `Duration`.
2. Stops when its time is up.
3. Looks at the damage actions in the plan (`Find` by ID). The first one aimed at the entity of its character it cancels, and it stops.
4. If there was none, sets itself for the next tick.

**Example.** A shield for 100 ticks is granted at tick 20. At tick 24 an attack adds a damage against its character: the shield runs first (priority -50 against 0), cancels the damage and is not set again. The character's health did not change, and the next damage lands.

### ZoneLayerCharacterRegenAction

**Where.** `ZoneLayerCharacterRegenAction` in `zonelayer_character_regen_action.go`; ID `ZoneLayerCharacterRegenActionID`; priority `PrioritySystem`; no author.

**Purpose.** Gives a little health to every living character.

**Reason.** The example of an action that runs on a cron and has no state.

**Context.** Runs in every zonelayer that was built with a regen period.

**Fields.** None.

**Schedule.** Every `RegenEvery` ticks, set in `actor.Spec` and registered by `actor.Build` with `AddCron`; 0 means never.

**What it does.** Calls `HealAllEntities` with 1 (written in the code): every character with health above 0 gets that much, up to its most, and the events get one change of health for each that changed.

**Example.** With `RegenEvery: 10` and a character at 90 of 100 health, the character is at 91 at tick 10, 92 at tick 20, and so on.

---

## Character enter and exit

### ZoneLayerCharacterEnterAction

**Where.** `ZoneLayerCharacterEnterAction` in `zonelayer_character_enter_action.go`; ID `ZoneLayerCharacterEnterActionID`; priority `PriorityResolve`; author: the character in the message (`Character.CharID`).

**Purpose.** Puts a character in the zonelayer: gives it a body at a position.

**Reason.** One way in for every character: a character that enters the world, a character that comes through an exit, and a character that is put back after it could not go, all arrive the same way.

**Context.** Needs the map, to see whether the position can be stood on and where the spawn point is, and reads it from the zone's published state. The character exists only in this message, so the action does not give it up.

**Fields.** `Character` (the whole character, health and cooldowns included), `At` (the position it asks for).

**Sent by.** `ZoneCharacterEnterAction` (a character entering the world), `ZoneLayerCharacterExitToAction` (a character coming through an exit, and the same action, sent back to its own zonelayer, as the compensation when the first message cannot be delivered).

**What it does.**
1. Does nothing if the character is already in the zonelayer.
2. Reads the zone's map. If the zone has not published yet, sets itself for the next tick and stops.
3. If the position asked for cannot be stood on, uses the spawn point of the map.
4. Calls `AttachCharacter`: the character gets a new entity.

**Example.** A character comes through the east exit of zone 1 asking for position (1, 2) in zone 2. The tile is free, so the character stands there, with the health and cooldowns it had.

### ZoneLayerCharacterExitToAction

**Where.** `ZoneLayerCharacterExitToAction` in `zonelayer_character_exit_to_action.go`; ID `ZoneLayerCharacterExitToActionID`; priority `PriorityIntent`; author: the character (`CharID`).

**Purpose.** A character takes the exit it stands in, to a zonelayer of the zone the exit leads to. The exit is optimistic: the character moves at once, when that zonelayer has room, and does not wait for it to answer.

**Reason.** The simplest way across zones: two actions, this one and the enter, and nothing kept between them. No scope holds a record of the character on the way, and no scope has to answer.

**Context.** Runs in the zonelayer that holds the character. It reads, from what is published and never from the other zonelayer itself:
- the map of its own zone, for the exit;
- the state of the zone the exit leads to, for its zonelayers and their capacity;
- the state of each of those zonelayers, for how many characters it holds.

What it reads may be a few ticks old, so two characters that go at the same tick may both find room for one, and a zonelayer may hold one character more than its capacity for a moment. When anything it needs is not published, or no zonelayer has room, the character stays where it is and nothing happens.

**Fields.** `CharID`, `ExitID` (the ID of the exit).

**Command.** `UseExitZoneLayerCommand{CharID, ExitID}`.

**On failure.** The message to the other zonelayer has a compensation: if it cannot be delivered (the zonelayer is gone, stopping, or its inbox is full), a `ZoneLayerCharacterEnterAction` with the same character comes back to this zonelayer, at the position the character stood at, and puts it back.

**What it does.**
1. Does nothing if the character is not in the zonelayer or has no health.
2. Reads the zone's map and finds the exit; does nothing if there is no such exit or the character does not stand in it.
3. Reads the state of the zone the exit leads to; does nothing if it is not published.
4. Takes the zonelayers of that zone in the order the zone has them, and picks the first that holds fewer characters than its capacity (one that published nothing counts as empty).
5. Calls `DetachCharacter`: the character leaves this zonelayer, whole.
6. Sends it to the zonelayer that was picked, in a `ZoneLayerCharacterEnterAction` that asks for the arrival position of the exit.

**Example.** A character stands on the east exit of zone 1. Zone 2 has one zonelayer of capacity 1 that holds nobody: the character leaves its zonelayer, and the other zonelayer puts it at (1, 2). If that zonelayer already held one character, nothing happens and the character stays on the exit. If the zonelayer was removed after it published, the message comes back and the character is where it stood.

---

## Chat

### ZoneLayerChatSendAction

**Where.** `ZoneLayerChatSendAction` in `zonelayer_chat_send_action.go`; ID `ZoneLayerChatSendActionID`; priority `PriorityResolve`; author: the character who said it (`FromID`).

**Purpose.** A message said in a zonelayer is heard in it.

**Reason.** The one place where a message becomes something the clients of a zonelayer hear. A character says it to its own zonelayer, and a zone gives it to each of its zonelayers.

**Context.** Does not need the map, and does not need the sender to be in this zonelayer: a message from the zone may be from a character that is in another one.

**Fields.** `FromID` (the character), `Text`.

**Command.** `ChatZoneLayerCommand{FromID, Text}` (a character saying something to its own zonelayer).

**Sent by.** `ZoneChatSendAction`, once for each zonelayer of the zone, dropped if it cannot be delivered.

**What it does.** Calls `SendMessage`: the message is kept in the zonelayer's chat table (the last 100) and the events get the message, tick included.

**Example.** At tick 5 character 1 says "hello zonelayer": the zonelayer records it and its clients are told. A zone message "hello zone" at tick 5 reaches zonelayer 1 and zonelayer 2 at tick 6.
