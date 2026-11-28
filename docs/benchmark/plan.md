# Benchmarks of the plan

Conventions and how to run: [index](index.md). Behavior: [`Plan`](../simulation.md#plan).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkPlanAppend/actions=N` | `platform/sim` | `Reset` and `Append` of `N` actions into a plan that is reused |
| `BenchmarkPlanAppendAndRun/actions=N` | `platform/sim` | the same, and taking every entry out with `Next` |
| `BenchmarkPlanBuildAndRun/actions=N` | `platform/sim` | `Build` of the `N` actions in one go (what an engine does with what a tick starts with), and `Next` of every entry |
| `BenchmarkPlanFindByID/actions=N` | `platform/sim` | `FindByID` over the same damage, all of it walked |
| `BenchmarkPlanFindByIDNested/actions=N` | `platform/sim` | `FindByID` of the damage and, inside it, of the regens, for the first 3 damage |

- The actions of a plan are moves (40%), attacks, damage and regens (20% each), the moves, attacks and regens of 200 characters; `N` is 20, 100 or 1,000.
- A plan is as large as the actions of one tick of one engine.

## Results

| Benchmark | N | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| PlanAppend | 20 | 1,265 (63 ns for each action) | 768 | 33 |
| PlanAppend | 100 | 6,340 (63 ns for each action) | 4,928 | 125 |
| PlanAppend | 1,000 | 64,300 (64 ns for each action) | 44,451 | 1,037 |
| PlanAppendAndRun | 20 | 1,340 | 768 | 33 |
| PlanAppendAndRun | 100 | 6,670 | 4,928 | 125 |
| PlanAppendAndRun | 1,000 | 67,500 | 44,451 | 1,037 |
| PlanBuildAndRun | 20 | 777 | 288 | 13 |
| PlanBuildAndRun | 100 | 3,590 | 2,528 | 25 |
| PlanBuildAndRun | 1,000 | 32,100 | 20,451 | 37 |
| PlanFindByID | 20 | 19 | 0 | 0 |
| PlanFindByID | 100 | 66 | 0 | 0 |
| PlanFindByID | 1,000 | 667 | 0 | 0 |
| PlanFindByIDNested | 20 | 55 | 0 | 0 |
| PlanFindByIDNested | 100 | 105 | 0 | 0 |
| PlanFindByIDNested | 1,000 | 752 | 0 | 0 |

The plan is a `table.PriorityQueue` whose entries keep their own status (pending, ran, cancelled, cut), with one column, for the ID of its actions. Taking out the column for the actor (the plan no longer has `Find`, and no code asked for it) took Append from 135 to 75 ns for each action, 16 to 30 percent fewer bytes and 40 to 50 percent fewer allocations; `FindByID` and the nested walk are as before. Before the status was in the entries, the queue kept an array of states and a list of what was cancelled: against it, Append and Build took the same time and 8 to 11 percent fewer bytes, and the nested walk was 4 to 10 percent faster. Before it was a sorted slice with two maps of buckets; the same benchmarks on that plan gave: Append 2,340, 14,143 and 176,287 ns (20, 100, 1,000), FindByID 29, 69 and 577 ns, FindByIDNested 103, 154 and 719 ns.

- `Append` keeps one column, for the ID of each action, and none for the actor or the Go type.
- **`Build` is about 40 to 50 percent faster than `Append` of each action and allocates about half the bytes and far fewer times,** because it orders the actions once and takes the entries from one array. An engine starts every tick with `Build`; `Append` is for what an action adds while the plan runs.
- **What the queue costs:** `FindByID` is 17 to 20 percent slower than on the sorted slice with two maps, at most 120 ns in 1,000 actions (the lookup goes through the column of the queue and reads each entry from its array); `Append` is faster than on that plan (36 to 59 percent faster).
- `FindByID` makes no slice, so it allocates nothing and a walk inside a walk costs no more than two walks.
- `FindByID` stops when the loop stops: the nested benchmark leaves the walk of the damage after 3 entries.
- `FindByID` is slower than a walk over a slice that the caller keeps and reuses (273 to 319 ns for 1,000 actions, measured while choosing it) and is not slower for 100 actions or fewer by more than 20 ns.
