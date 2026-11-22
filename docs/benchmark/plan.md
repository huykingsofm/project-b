# Benchmarks of the plan

Conventions and how to run: [index](index.md). Behavior: [`Plan`](../simulation.md#plan).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkPlanAppend/actions=N` | `platform/sim` | `Reset` and `Append` of `N` actions into a plan that is reused, as an engine does every tick |
| `BenchmarkPlanFind/actions=N` | `platform/sim` | `Find` of the damage by its ID in a plan of `N` actions: makes a slice |
| `BenchmarkPlanFindByID/actions=N` | `platform/sim` | `FindByID` over the same damage, all of it walked |
| `BenchmarkPlanFindByIDNested/actions=N` | `platform/sim` | `FindByID` of the damage and, inside it, of the regens, for the first 3 damage |

- The actions of a plan are moves (40%), attacks, damage and regens (20% each), the moves, attacks and regens of 200 characters; `N` is 20, 100 or 1,000.
- A plan is as large as the actions of one tick of one engine.

## Results

| Benchmark | N | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| PlanAppend | 20 | 2,430 (122 ns for each action) | 960 | 57 |
| PlanAppend | 100 | 15,000 (150 ns for each action) | 5,888 | 245 |
| PlanAppend | 1,000 | 194,000 (194 ns for each action) | 65,959 | 2,117 |
| PlanFind | 20 | 74 | 56 | 3 |
| PlanFind | 100 | 251 | 504 | 6 |
| PlanFind | 1,000 | 1,612 | 4,472 | 9 |
| PlanFindByID | 20 | 29 | 0 | 0 |
| PlanFindByID | 100 | 71 | 0 | 0 |
| PlanFindByID | 1,000 | 607 | 0 | 0 |
| PlanFindByIDNested | 20 | 105 | 0 | 0 |
| PlanFindByIDNested | 100 | 155 | 0 | 0 |
| PlanFindByIDNested | 1,000 | 690 | 0 | 0 |

- `Append` keeps one index for the ID and one for the actor of each action, and none for the Go type.
- `FindByID` makes no slice, so it allocates nothing and a walk inside a walk costs no more than two walks; `Find` makes a slice for each call.
- `FindByID` stops when the loop stops: the nested benchmark leaves the walk of the damage after 3 entries.
- `FindByID` is slower than a walk over a slice that the caller keeps and reuses (273 to 319 ns for 1,000 actions, measured while choosing it) and is not slower for 100 actions or fewer by more than 20 ns.
