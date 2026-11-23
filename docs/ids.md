# IDs that a state gives out

`shared/ids`. An allocator gives out the IDs of things that come and go while the game runs, and keeps an ID that was given back in a grave for a number of steps, so that what still points at the old one has had the time to let go.

## What it does

| Call | What it does |
|---|---|
| `ids.New(steps, limit)` | makes an allocator that gives the IDs from 1 to `limit`, and keeps an ID that was given back in the grave for `steps` steps |
| `Give(inUse, inUseCount)` | returns the next ID after the last one given that is neither in use nor in the grave, going round to 1 after the limit; `inUse` and `inUseCount` are what the owner says of the IDs it uses; false when there is none |
| `Release(id)` | gives the ID back: it waits in the grave; false for an ID that cannot have been given, or that is in the grave already, which changes nothing |
| `Step()` | takes every ID in the grave one step; the ones that have waited the steps leave it |
| `Status(id, inUse)` and `InGrave(id)` | say what an ID is (below) |
| `MayBeInUse(id)` | whether the owner can be using the ID: it was given and is not in the grave; what a checkpoint is checked with |
| `Clone()` | a copy that shares nothing |
| `Encode(w)` and `ids.Decode[T](r)` | write and read it, for a checkpoint |
| `Steps`, `Limit`, `Last`, `Wrapped`, `Clock`, `Grave`, `GraveLen` | what it holds, in a fixed order, for the hash of a state |

## What an ID is

| Status | Meaning |
|---|---|
| `Live` | the owner uses it |
| `InGrave` | it was given back and has not waited the steps yet |
| `NotGiven` | neither: never given, given back and out of the grave, or 0, which is never given |

## What it holds

| Held | Why |
|---|---|
| the steps and the limit | what it was made with |
| the last ID given, and whether it went round to 1 | where the next one is looked for; until it goes round, every ID above the last was never given, so none is checked |
| the clock, and the grave in the order the IDs were given back, with the step at which each leaves | what waits |

- It does not hold which IDs are in use, and it does not hold a list of the IDs that left the grave: the owner knows the first (a state has its entities), and the second is the same as an ID never given. So it is as small as its grave, whatever was given before.
- The IDs are given in order up to the limit before one is given again, so an ID comes back only after the allocator went round, and after its steps.

## Rules

- 0 is never given.
- With `steps` of 0 nothing waits: an ID that is given back is not given at once all the same, as the next one is looked for after the last one given.
- After the allocator went round, `Give` skips the IDs in use and in the grave; it looks at no more than the limit of them, and reports false at once when `inUseCount` and the grave fill the limit.
- It is not safe for concurrent use: the state that owns it is changed by one goroutine.
- A call with the same arguments in the same order ends with the same allocator, so a replay of the same inputs gives the same IDs.
- `Decode` refuses a count that claims more than the bytes left can hold, a last ID above the limit, and an ID in the grave that is 0, never given, there twice, or whose step is not after the clock and the ones before it.

## Who uses it

| Who | What it gives out | Steps of the grave |
|---|---|---|
| `zonelayer` and `zone` states | the local number of the `EntityID` of each entity, from 1 up to 2^32 - 1; an entity that leaves (a body, an item) gives its number back | set when the state is made (`ClusterBuilder.WithEntityGraveSteps`, 12000 ticks unless set), written in the checkpoint with the allocator |
| `world` state | the local number of the `StateID` of each zonelayer, from 1 up to 2^24 - 1 (`State.AddZoneLayer` gives the ID and makes the world know the zonelayer; `RemoveZoneLayer` gives the number back) | set when the state is made (`ClusterBuilder.WithZoneLayerGraveSteps`, 12000 ticks unless set), written in the checkpoint with the allocator |

- The IDs of zones come from the content (the map of a zone says which zone an exit leads to), so no allocator gives them.
- The state steps its allocator once in `SetTick`, at the start of each tick, so the grave is counted in the ticks of the state and a replay counts the same.
- The steps must be longer than anything that can still name the entity: the removal that is set for an item on the ground (`ZoneLayerItemEntityRemoveAction`, at the tick the item expires) holds its `EntityID`, and an ID that came back sooner would have that action remove another item. The same is true of the ID of a zonelayer and a message that waits to be sent to it.
- A state whose allocator has no ID left refuses the new entity (`AddCharacterEntity` and `AddItemEntity` return false), and the action logs an error. With 2^32 IDs and the ones that left coming back, only a state that holds that many entities at once, or that gave them away faster than they wait, can reach it.
- A checkpoint is refused when an entity it holds (or a zonelayer the world knows) has an ID that its allocator cannot have given: of another owner, never given, or in the grave. The hash of a state covers the allocator, the grave included.
