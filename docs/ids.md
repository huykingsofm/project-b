# IDs that a state gives out

`shared/ids`. An allocator gives out the IDs of things that come and go while the game runs, and keeps an ID that was given back in a grave for a number of steps, so that what still points at the old one has had the time to let go.

## What it does

| Call | What it does |
|---|---|
| `ids.New(steps, limit)` | makes an allocator that gives the IDs from 1 to `limit`, and keeps an ID that was given back in the grave for `steps` steps |
| `Give()` | returns an ID that is live from now on: the next one after the last one given that is neither live nor in the grave, going round to 1 after the limit; false when there is none |
| `Release(id)` | gives a live ID back: it waits in the grave; false for an ID that is not live, which changes nothing |
| `Step()` | takes every ID in the grave one step; the ones that have waited the steps leave it |
| `Status(id)`, `IsLive(id)`, `InGrave(id)` | say what an ID is (below) |
| `LiveIDs()`, `LiveCount()` | the live IDs, from the smallest, and how many there are |
| `Clone()` | a copy that shares nothing |
| `Encode(w)` and `ids.Decode[T](r)` | write and read it, for a checkpoint |
| `Steps`, `Limit`, `Last`, `Wrapped`, `Clock`, `Grave`, `GraveLen` | what it holds, for the hash of a state |

## What an ID is

| Status | Meaning |
|---|---|
| `Live` | it was given and not given back |
| `InGrave` | it was given back and has not waited the steps yet |
| `NotGiven` | neither: never given, given back and out of the grave, or 0, which is never given |

## What it holds

| Held | Why |
|---|---|
| the steps and the limit | what it was made with |
| the last ID given, and whether it went round to 1 | where the next one is looked for; until it goes round, every ID above the last was never given, so none is checked |
| the live IDs | which IDs are given and not given back, to skip them after it went round and to refuse giving back one that is not live |
| the clock, and the grave in the order the IDs were given back, with the step at which each leaves | what waits |

- It holds no list of the IDs that left the grave: such an ID is the same as one never given. So it is as big as what is live and what waits, whatever was given before.
- The IDs are given in order up to the limit before one is given again, so an ID comes back only after the allocator went round, and after its steps.

## Rules

- 0 is never given.
- With `steps` of 0 nothing waits: an ID that is given back is not given at once all the same, as the next one is looked for after the last one given.
- `Give` reports false at once when the live IDs and the grave fill the limit; otherwise it finds one.
- It is not safe for concurrent use: the state that owns it is changed by one goroutine.
- A call with the same arguments in the same order ends with the same allocator, so a replay of the same inputs gives the same IDs.
- `Encode` writes the live IDs from the smallest, so the same allocator is always the same bytes.
- `Decode` refuses a count that claims more than the bytes left can hold, a last ID above the limit, an ID that is 0, never given or held twice (live twice, in the grave twice, or both), an ID in the grave whose step is not after the clock and the ones before it, and more IDs held than the limit.

## Who uses it

| Who | What it gives out | Steps of the grave |
|---|---|---|
| `zonelayer` and `zone` states | the local number of the `EntityID` of each entity, from 1 up to 2^32 - 1; an entity that leaves (a body, an item) gives its number back | set when the state is made (`ClusterBuilder.WithEntityGraveSteps`, 12000 ticks unless set), written in the checkpoint with the allocator |
| `world` state, with `sim.StateIDs` | the `StateID` of each state it makes, from an allocator for each type, local numbers from 1 up to 2^24 - 1; today the zonelayers (`State.AddZoneLayer` gives the ID and makes the world know the zonelayer; `RemoveZoneLayer` gives it back) | set when the state is made (`ClusterBuilder.WithStateGraveSteps`, 12000 ticks unless set), the same for every type, written in the checkpoint with the allocators |

- The IDs of zones come from the content (the map of a zone says which zone an exit leads to), so no allocator gives them.
- The state steps its allocator once in `NextTick`, at the start of each tick, so the grave is counted in the ticks of the state and a replay counts the same.
- The steps must be longer than anything that can still name the entity: the removal that is set for an item on the ground (`ZoneLayerItemEntityRemoveAction`, at the tick the item expires) holds its `EntityID`, and an ID that came back sooner would have that action remove another item. The same is true of the ID of a zonelayer and a message that waits to be sent to it.
- A state whose allocator has no ID left refuses the new entity (`AddCharacterEntity` and `AddItemEntity` return false), and the action logs an error. With 2^32 IDs and the ones that left coming back, only a state that holds that many entities at once, or that gave them away faster than they wait, can reach it.
- A checkpoint is refused when the entities a state holds (or the zonelayers the world knows) are not exactly the live IDs of its allocator. The hash of a state covers the allocator and its grave; the live IDs are not in it again, as the hash of the state covers what it holds.
