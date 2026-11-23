# Benchmarks of the plan

Conventions and how to run: [index](index.md). Behavior: [`Plan`](../simulation.md#plan).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkPlanAppend/actions=N` | `platform/sim` | `Reset` and `Append` of `N` actions into a plan that is reused |
| `BenchmarkPlanAppendAndRun/actions=N` | `platform/sim` | the same, and taking every entry out with `Next` |
| `BenchmarkPlanBuildAndRun/actions=N` | `platform/sim` | `Build` of the `N` actions in one go (what an engine does with what a tick starts with), and `Next` of every entry |
| `BenchmarkPlanFind/actions=N` | `platform/sim` | `Find` of the damage by its ID in a plan of `N` actions: makes a slice |
| `BenchmarkPlanFindByID/actions=N` | `platform/sim` | `FindByID` over the same damage, all of it walked |
| `BenchmarkPlanFindByIDNested/actions=N` | `platform/sim` | `FindByID` of the damage and, inside it, of the regens, for the first 3 damage |

- The actions of a plan are moves (40%), attacks, damage and regens (20% each), the moves, attacks and regens of 200 characters; `N` is 20, 100 or 1,000.
- A plan is as large as the actions of one tick of one engine.

## Results

| Benchmark | N | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| PlanAppend | 20 | 2,710 (135 ns for each action) | 960 | 57 |
| PlanAppend | 100 | 13,500 (135 ns for each action) | 5,888 | 245 |
| PlanAppend | 1,000 | 124,800 (125 ns for each action) | 63,658 | 2,117 |
| PlanAppendAndRun | 20 | 2,780 | 960 | 57 |
| PlanAppendAndRun | 100 | 13,900 | 5,888 | 245 |
| PlanAppendAndRun | 1,000 | 128,400 | 63,659 | 2,117 |
| PlanBuildAndRun | 20 | 2,300 | 480 | 37 |
| PlanBuildAndRun | 100 | 11,000 | 3,488 | 145 |
| PlanBuildAndRun | 1,000 | 90,100 | 39,659 | 1,117 |
| PlanFind | 20 | 75 | 56 | 3 |
| PlanFind | 100 | 244 | 504 | 6 |
| PlanFind | 1,000 | 1,439 | 4,472 | 9 |
| PlanFindByID | 20 | 31 | 0 | 0 |
| PlanFindByID | 100 | 81 | 0 | 0 |
| PlanFindByID | 1,000 | 685 | 0 | 0 |
| PlanFindByIDNested | 20 | 108 | 0 | 0 |
| PlanFindByIDNested | 100 | 156 | 0 | 0 |
| PlanFindByIDNested | 1,000 | 820 | 0 | 0 |

The plan is a `table.PriorityQueue` whose entries keep their own status (pending, ran, cancelled, cut): the queue keeps no array of states and no list of what was cancelled. Against the queue that kept them, Append and Build take the same time and 8 to 11 percent fewer bytes, `FindByID` takes the same time (it no longer checks a state, and gives every entry), and the nested walk is 4 to 10 percent faster. Before it was a sorted slice with two maps of buckets; the same benchmarks on that plan gave: Append 2,340, 14,143 and 176,287 ns (20, 100, 1,000), FindByID 29, 69 and 577 ns, FindByIDNested 103, 154 and 719 ns, Find the same as now, and 14 percent fewer bytes and the same allocations for Append (the columns are slices of keys, as the buckets were; the queue keeps one more array, of the states of its elements).

- `Append` keeps one column for the ID and one for the actor of each action, and none for the Go type.
- **`Build` is about 20 to 30 percent faster than `Append` of each action and allocates half as much,** because it orders the actions once and takes the entries from one array. An engine starts every tick with `Build`; `Append` is for what an action adds while the plan runs.
- **What the queue costs:** `FindByID` is 17 to 20 percent slower than on the sorted slice with two maps, at most 120 ns in 1,000 actions (the lookup goes through the column of the queue and reads each entry from its array); `Append` is no slower (faster at 1,000, 13 percent slower at 20).
- `FindByID` makes no slice, so it allocates nothing and a walk inside a walk costs no more than two walks; `Find` makes a slice for each call.
- `FindByID` stops when the loop stops: the nested benchmark leaves the walk of the damage after 3 entries.
- `FindByID` is slower than a walk over a slice that the caller keeps and reuses (273 to 319 ns for 1,000 actions, measured while choosing it) and is not slower for 100 actions or fewer by more than 20 ns.
