# IDs that a state gives out

`shared/ids`. An allocator gives out the IDs of things that come and go while the game runs, and keeps an ID that was given back in a grave for a number of steps, so that what still points at the old one has had the time to let go.

## What it does

| Call | What it does |
|---|---|
| `ids.New(steps, limit)` | makes an allocator that gives the IDs from 1 to `limit`, and keeps an ID that was given back in the grave for `steps` steps |
| `Give()` | returns an ID that is live from now on: the one that has been free longest, or else the next one never given; false when there is none |
| `Release(id)` | gives the ID back: it waits in the grave, and is then free; false for an ID that is not live, which changes nothing |
| `Step()` | takes every ID in the grave one step; the ones that have waited the steps are free |
| `Status(id)` | says what an ID is (below) |
| `Clone()` | a copy that shares nothing |
| `Encode(w)` and `ids.Decode[T](r)` | write and read it, for a checkpoint |
| `Steps`, `Limit`, `Largest`, `Clock`, `LiveCount`, `Grave`, `FreeIDs` | what it holds, in a fixed order, for the hash of a state |

## What an ID is

| Status | Meaning |
|---|---|
| `NeverGiven` | the allocator never gave it, or it is 0, which is never given |
| `Live` | it was given and not given back |
| `InGrave` | it was given back and has not waited the steps yet |
| `Free` | it was given back and has waited: it is given again before any new one |

## Rules

- 0 is never given. An ID that is given back is given again only after its steps, and the one free longest goes first, so the time between two lives of an ID is as long as it can be.
- With `steps` of 0 an ID is free as soon as it is given back.
- It is not safe for concurrent use: the state that owns it is changed by one goroutine.
- A call with the same arguments in the same order ends with the same allocator, so a replay of the same inputs gives the same IDs.
- `Decode` refuses a count that claims more than the bytes left can hold, an ID that is 0, above the largest that was given or in two places, and an ID in the grave before its step.

## Who uses it

| Who | What it gives out | Steps of the grave |
|---|---|---|
| `zonelayer` and `zone` states | the local number of the `EntityID` of each entity, from 1 up to 2^32 - 1; an entity that leaves (a body, an item) gives its number back | set when the state is made (`ClusterBuilder.WithEntityGraveSteps`, 12000 ticks unless set), written in the checkpoint with the allocator |

- The state steps its allocator once in `SetTick`, at the start of each tick, so the grave is counted in the ticks of the state and a replay counts the same.
- The steps must be longer than anything that can still name the entity: the removal that is set for an item on the ground (`ZoneLayerItemEntityRemoveAction`, at the tick the item expires) holds its `EntityID`, and an ID that came back sooner would have that action remove another item.
- A state whose allocator has no ID left refuses the new entity (`AddCharacterEntity` and `AddItemEntity` return false), and the action logs an error. With 2^32 IDs and the ones that left coming back, only a state that holds that many entities at once, or that gave them away faster than they wait, can reach it.
- A checkpoint is refused when the entities it holds are not exactly the live IDs of its allocator, and the hash of a state covers the allocator, the grave and the free list included.
