# IDs of states and entities

`platform/sim` (`StateID`) and `server/game/sim/state` (`EntityID`). The ID of a state and of an entity is counted up and never given again, so it names one thing for good: in a log, in a checkpoint, in a message that waits, in an action that is set for later.

## What an ID is

| ID | Made of | Zero value |
|---|---|---|
| `sim.StateID` (`uint64`) | the `StateType` in the 8 high bits, and in the 56 low bits a number that is local to the type, counted from 1 | no state |
| `state.EntityID` (struct) | `Owner`, the `StateID` of the state that holds the entity, and `Local`, a `uint64` that the owner counted from 1 | no entity: `Decode` refuses it |

| Call | What it does |
|---|---|
| `sim.NewStateID(type, local)` | makes the ID from a number of at most `sim.MaxLocalStateID` (2^56 - 1); `Type()` and `Local()` read it back |
| `state.NewEntityID(owner, local)` | makes the ID; `Owner` and `Local` are its fields, `String()` says them for a log |
| `EntityID.Encode(w)` | writes the owner and then the number, each as a uvarint |
| `state.DecodeEntityID(r)` | reads what `Encode` wrote; an error (`ErrNoEntity`) for an ID with no owner or no number |
| `EntityID.Hash(h)` | works the owner and the number into a hash |

## Who gives the numbers

There is no counter type: a state that gives IDs holds the last number it gave, as a plain field, which is written in its checkpoint and worked into its hash.

| State | Field | What it gives |
|---|---|---|
| `world` | `lastStateID` | the `StateID` of each zonelayer it makes (`State.AddZoneLayer`; `RemoveZoneLayer` makes it forget the zonelayer) |
| `zonelayer` | `lastEntityID` | the `EntityID` of each entity: a body, an item (`AddCharacterEntity`, `AddItemEntity`) |
| `zone` | `lastEntityID` | the `EntityID` of each exit of its map, when the state is made |

## Rules

- 0 is never given: a number is counted up from 1 and never given again. Nothing checks that a number is left, since 56 bits for a type and 64 for an owner are not reached in practice (at a thousand a second, 2 million years for a type), and nothing is done for a number that goes round.
- A call with the same arguments in the same order ends with the same field, so a replay of the same inputs gives the same IDs.
- When a state is read from a checkpoint, each ID it holds is checked against the field: a zonelayer of the world, or an entity of a state, whose owner is not the state, or whose number is 0 or above the number last given, makes the checkpoint an error, and so does a number that is too large for the ID of a state.
- A state writes its own entities with the number only: it is the owner of all of them, and says so when it reads them. An `EntityID` that goes from a state to another (in a command, an action or an event) is written whole, with `Encode`.
- Nothing waits for an ID to be free: what names an ID that no longer names anything (an action that removes an item at the tick it expires, a message that waits to be sent to a zonelayer that was removed) finds that nothing is there, and does nothing, or bounces as `DeliveryFailureRetired`.
- After a crash the log of each engine is cut (see [replay log](replay_log.md#recovery-line)) and a state is made again from its checkpoint, so the field goes back to the value it had at that tick, and an ID that was given at a tick that was cut is given again, to another thing. It is safe because whatever depends on that tick is cut with it. A log that is not of an engine that is live after the cut must not be left under that ID: when engines are made while the game runs, either the making of an engine depends on a tick of the world that gave its ID, or the logs of the IDs above `lastStateID` of the world are cut to nothing (the roadmap has it).

- The IDs of zones come from the content (the map of a zone says which zone an exit leads to), so the world does not give them.
