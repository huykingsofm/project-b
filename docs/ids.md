# IDs of states and entities

`platform/sim` (`StateID`, `StateIDCounters`) and `server/game/sim/state` (`EntityID`, `EntityIDCounter`). The ID of a state and of an entity is counted up and never given again, so it names one thing for good: in a log, in a checkpoint, in a message that waits, in an action that is set for later.

## What an ID is

| ID | Made of | Zero value |
|---|---|---|
| `sim.StateID` (`uint64`) | the `StateType` in the 8 high bits, and in the 56 low bits a number that is local to the type, counted from 1 | no state |
| `state.EntityID` (struct) | `Owner`, the `StateID` of the state that holds the entity, and `Local`, a `uint64` that the owner counted from 1 | no entity: `Decode` refuses it |

| Call | What it does |
|---|---|
| `sim.NewStateID(type, local)` | makes the ID; `Type()` and `Local()` read it back |
| `state.NewEntityID(owner, local)` | makes the ID; `Owner` and `Local` are its fields, `String()` says them for a log |
| `EntityID.Encode(w)` | writes the owner and then the number, each as a uvarint |
| `state.DecodeEntityID(r)` | reads what `Encode` wrote; an error (`ErrNoEntity`) for an ID with no owner or no number |
| `EntityID.Hash(h)` | works the owner and the number into a hash |

## Counters

| Counter | Call | What it does |
|---|---|---|
| `sim.StateIDCounters` | `Give(type)` | returns the next ID of the type, from 1; false when the 56 bits of the type are all given |
| | `Last(type)` | the number last given for the type, or 0 |
| | `Clone()`, `Hash(h)` | a copy that shares nothing, and what goes into the hash of the state |
| | `Encode(w)`, `sim.DecodeStateIDCounters(r)` | write and read it for a checkpoint; refuses a type twice or out of order, a number that is 0 or more than 56 bits hold, and more types than bytes |
| `state.EntityIDCounter` | `Give()` | returns the next entity ID of the owner, from 1; false when the 64 bits are all given |
| | `Last()`, `Owner()` | the number last given, or 0; the owner |
| | `CheckGiven(id)` | an error for an ID of another state, or a number that is 0 or not counted to yet |
| | `Clone()`, `Hash(h)`, `Encode(w)`, `state.DecodeEntityIDCounter(owner, r)` | as above; the owner is not written, the state that holds it knows it |

## Rules

- 0 is never given, and a counter that has no number left refuses and does not go round: a state that cannot give an ID refuses the new entity (`AddCharacterEntity` and `AddItemEntity` return false, and the action logs an error). With 56 bits for a type and 64 for an owner, this is not reached in practice.
- A call with the same arguments in the same order ends with the same counter, so a replay of the same inputs gives the same IDs.
- A counter is plain data that a state holds, writes to its checkpoint and works into its hash. A checkpoint holds the last number of each counter, and the IDs that are live once: in what the owner holds (its entities, its zonelayers). When a state is read, each of them is checked against the counter (`CheckGiven`, and for the world the zonelayers against `StateIDCounters.Last`); an ID of another state or type, never given or there twice makes the checkpoint an error.
- A state writes its own entities with the number only: it is the owner of all of them, and says so when it reads them. An `EntityID` that goes from a state to another (in a command, an action or an event) is written whole, with `Encode`.
- Nothing waits for an ID to be free: what names an ID that no longer names anything (an action that removes an item at the tick it expires, a message that waits to be sent to a zonelayer that was removed) finds that nothing is there, and does nothing, or bounces as `DeliveryFailureRetired`.
- After a crash the log of each engine is cut (see [replay log](replay_log.md#recovery-line)) and a state is made again from its checkpoint, so the counter goes back to the value it had at that tick, and an ID that was given at a tick that was cut is given again, to another thing. It is safe because whatever depends on that tick is cut with it. A log that is not of an engine that is live after the cut must not be left under that ID: when engines are made while the game runs, either the making of an engine depends on a tick of the world that gave its ID, or the logs of the IDs above the counter of the world are cut to nothing (the roadmap has it).

## Who uses it

| Who | What it gives out |
|---|---|
| `zonelayer` and `zone` states, with `state.EntityIDCounter` | the `EntityID` of each entity: the body of a character, an item on the ground, an exit; an entity that leaves leaves its ID with it |
| `world` state, with `sim.StateIDCounters` | the `StateID` of each state it makes, a counter for each type; today the zonelayers (`State.AddZoneLayer` gives the ID and makes the world know the zonelayer; `RemoveZoneLayer` makes it forget it) |

- The IDs of zones come from the content (the map of a zone says which zone an exit leads to), so no counter gives them.
- The world says what a `StateID` is (`StateStatus`, the `sim.StateDirectory` that the router asks): `StateLive` for itself, a zone it knows and a zonelayer it knows; `StateRemoved` for a zonelayer whose number was given and that it does not know; `StateNotGiven` for any other ID. Removed stays removed.
