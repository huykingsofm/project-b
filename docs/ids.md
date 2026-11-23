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
