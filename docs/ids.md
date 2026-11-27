# IDs of states and entities

`platform/sim` (`StateID`) and `server/game/sim/state` (`EntityID`). The ID of a state and of an entity is counted up and never given again, so it names one thing for good: in a log, in a checkpoint, in a message that waits, in an action that is set for later. A state does not hold its own ID: it is the engine's, in the header of a checkpoint and in `Context.Self`.

## What an ID is

| ID | Made of | Zero value |
|---|---|---|
| `sim.StateID` (`uint64`) | the `StateType` in the 8 high bits, and in the 56 low bits a number that is local to the type, counted from 1 | no state |
| `state.EntityID` (`uint64`) | a number that the state that holds the entity counted from 1; it names an entity among those of its own state, not among those of another | no entity |

| Call | What it does |
|---|---|
| `sim.NewStateID(type, local)` | makes the ID from a number of at most `sim.MaxLocalStateID` (2^56 - 1); `Type()` and `Local()` read it back |
| `EntityID.Encode(w)` | writes the number as a uvarint |
| `state.DecodeEntityID(r)` | reads what `Encode` wrote |
| `EntityID.Hash(h)` | works the number into a hash |

## Who gives the numbers

The cluster gives the `StateID` of an engine that has none given (`Cluster.NewStateID`, [engines](engine_lifecycle.md)): it counts the numbers of each type up from 1, and when it starts again it counts from above the highest that its logs hold. The world knows the zones and the zonelayers by the IDs that the cluster gave, which it takes from the answer of the cluster (`State.HandleDirective`, `AddZone`, `AddZoneLayer`, `RemoveZoneLayer`). There is no counter type for entities: a state that gives IDs holds the last number it gave, as a plain field, which is written in its checkpoint and worked into its hash.

| State | Field | What it gives |
|---|---|---|
| `zonelayer` | `lastEntityID` | the `EntityID` of each entity: a body, an item (`AddCharacterEntity`, `AddItemEntity`) |
| `zone` | `lastEntityID` | the `EntityID` of each exit that is put in it (`AddExit`) |

## Rules

- 0 is never given: a number is counted up from 1 and never given again. Nothing checks that a number is left, since 56 bits for a type and 64 for an owner are not reached in practice (at a thousand a second, 2 million years for a type), and nothing is done for a number that goes round.
- A call with the same arguments in the same order ends with the same field, so a replay of the same inputs gives the same IDs.
- When a state is read from a checkpoint, an entity is taken as it is written: nothing checks its number against the field. A zonelayer that the world lists must be one of its type, with a number above 0, and listed once.
- An `EntityID` names an entity of the state that holds it, and of no other. A command or an action that is to name an entity of another state must hold the ID of that state beside it (0 for the state it runs on); none does now, since the exit of a zone is read through the zone of the zonelayer. An event holds the number only, and the emitter, which is told which engine made it, says whose it is.
- Nothing waits for an ID to be free: what names an ID that no longer names anything (an action that removes an item at the tick it expires, a message that waits to be sent to a zonelayer that was removed) finds that nothing is there, and does nothing, or bounces as `DeliveryFailureRetired`.
- After a crash the log of each engine is cut (see [event log](eventlog.md#recovery-line)) and a state is made again from its checkpoint, so the field of an entity counter goes back to the value it had at that tick, and an entity ID that was given at a tick that was cut is given again, to another thing. It is safe because whatever depends on that tick is cut with it. The `StateID` of an engine is not given again by that: the cluster counts from above the highest number that its logs hold, a log that was cut to nothing included, and the first record of the log of an engine that an action asked for depends on the tick that asked.

- An exit names the zone it leads to by its `StateID`. The world puts it in only when it lists both zones, so the number is one that the cluster gave.
