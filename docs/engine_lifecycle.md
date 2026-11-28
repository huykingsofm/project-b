# Making and ending engines

`platform/runtime/engine` (`engine.go`, `peer.go`, `engine_registry.go`, `environment.go`), `platform/runtime/router` (`directive_router.go`), `platform/runtime/cluster` (`cluster.go`, `orchestrator.go`, `stepping`), `platform/sim` (`directive.go`, `registry.go`, `coordinator.go`), `platform/journal/eventlog` (`Init`, `Started`, `Directive`, `Retired`), `platform/journal/replay` (`plan.go`, `replay.go`, `run.go`), `server/game` (`sim/action` and `sim/command`: `WorldZoneAdd`, `WorldZoneLayerAdd`, `ZoneLayerAnnounceToZone`, `ZoneZoneLayerAdd`). The design is in [proposed-designs](../plan/en/proposed-designs.md#making-and-ending-engines).

## Who is responsible

| What | Who |
|---|---|
| the `StateID` of an engine | the orchestrator of the cluster (`Orchestrator.NextStateID(type)`): the numbers of a type count up from 1 and are never given again. A state does not hold its ID: an action reads it from `Context.Self` |
| the coordinator engine | `cluster.NewCluster` makes it; it is the only engine that no request made |
| every other engine | the cluster, by the request of an engine; the engines of the game are asked for by the coordinator engine, by `sim.CreateEngineCommand` |
| the state and what the engine starts with | the one that asks: the request holds the state (without an ID) and the actions that run once, at the tick 2 of the engine (one that is to run again sets itself again) |
| how an engine is made from a state of a type | `engine.EngineRegistry`, which the game fills with one line for each type of state (`server/game/codecs.go`) |
| which engines a state lists | the state: it takes the response of the cluster (`sim.DirectiveHandler`) |

## The directive router

`router.DirectiveRouter` holds, behind one mutex, a list of requests, a list of acknowledgements, and a list of responses for each engine. Nothing is dropped because a list is full. Each side has the interface it needs (`EngineDirectives`, `ClusterDirectives`).

| Struct | Holds |
|---|---|
| `sim.DirectiveRequest` | `ID` (the number the engine gave it, from 1), `Tick` (its tick when it made it), `Kind` (`DirectiveCreateEngine`, `DirectiveRetireEngine`), `Tag`, `State` and `StartActions` (to make), `Target` (to end) |
| `sim.DirectiveResponse` | `ID`, `Kind`, `Tag`, `Engine` (the ID that was made or ended), `Tick` (for an ending: the tick of the `Retired` frame), `Failure` (empty for success) |
| `router.ResponseAcknowledgement` | the engine, the number of the request, and the tick at which it took the response |

- An action asks with `ActionRegistry.RequestCreateEngine(tag, state, startActions...)` or `ActionRegistry.RequestRetireEngine(id, tag)`. The engine numbers the requests when the tick is done, keeps them in its own data (`peerData`: in its checkpoint and its hash) and publishes them to the router. An engine made again from a checkpoint publishes the requests that wait at its first tick, since what the router held was lost.
- The engine takes its responses first in its tick (`takeDirectiveResponses`), writes each to the log (`eventlog.Directive`) and gives it to the state (`HandleDirective`). A response to a request that does not wait is dropped with no trace. A state that is not a `DirectiveHandler` gets a line of data to fix in the log.
- When an engine leaves the router (`peer.leaveRouters`) what waits for it, and the requests it published that no one took, are let go. The acknowledgements it published stay: an engine that took the response and then retired at once still lets the engine it made run.

## The periodic task

`Cluster.RunPeriodicTasks` is called by `Cluster.Run` on a ticker and by `stepping.Step` before each step, from one goroutine at a time. It does, in this order:

1. sends the coordinator engine what the configuration store holds (`Orchestrator.SyncConfig`);
2. takes the engines whose creators acknowledged into the cluster (`Engine.Adopt`);
3. ends the engines whose creator is gone and did not acknowledge (`Engine.Retire`): an acknowledgement published before the creator left was taken at 2;
4. serves the requests, in the order they were published;
5. answers the requests to end an engine whose engine has closed.

The orchestrator is called only from here, so it takes no lock.

## Making an engine

A@100 asks, B is made, A takes the response at its tick 103.

| Step | What | Log |
|---|---|---|
| 1 | the request is published, with the tick 100 | |
| 2 | the cluster takes an ID from the orchestrator, makes B with `EngineRegistry.Make` and runs its tick 1 itself (outside the router, on no goroutine); the actions it starts with are for the tick 2 and later | B@1: `Init{Creator, CreatorTick 100, DirectiveID, Snapshot}`, which depends on A@100 |
| 3 | the cluster publishes the response with the ID of B | |
| 4 | A takes it at the start of its tick 103 | A@103: `Directive`, which depends on B@1 |
| 5 | at the end of the tick, after its copy is published, A publishes the acknowledgement (the request, the tick 103) | |
| 6 | the cluster puts B in the router (`Engine.Adopt`), adds it to the list, and `Run` runs it from now on | B@2: `Started{Creator, ResponseTick 103}`, which depends on A@103 |

- A failure (a type that the registry does not know, an action that is not one of the state, a state that cannot be written) makes no engine and answers with the failure.
- If the creator is gone before it acknowledges, the cluster retires B: its log holds `Init` and `Retired`.
- A zonelayer announces itself to its zone by an action it starts with (`ZoneLayerAnnounceToZone`): only the zonelayer knows its ID by then.

## Ending an engine

| Step | What | Log |
|---|---|---|
| 1 | an action asks (`ActionRegistry.RequestRetireEngine`) or the coordinator engine takes `sim.RetireEngineCommand` | |
| 2 | the cluster asks B to retire (`Engine.RequestRetirement(by, byTick)`) and keeps the request until B is closed | |
| 3 | B retires between two of its ticks | `Retired{By, ByTick}`, which depends on the tick of the engine that asked |
| 4 | the cluster publishes the response with the tick of `Retired` | |
| 5 | A takes it | `Directive`, which depends on the `Retired` of B |

- An engine that asks for itself to end gets no response, and its `Retired` has no `By`.
- A request for an engine that is not there answers with a failure; the coordinator engine is never ended.
- `Cluster.RequestEngineRetirement(id)` asks from outside, and `stepping.RetireEngine(c, id)` asks and retires at once, between two steps, for a cluster that `Run` does not run.
- `Engine.Retire` writes the last frame of the log and then cleans up what the engine registered (`cleanUpRegistrations`): what it published goes, its queue goes back to its senders, and a message to it comes back as `DeliveryFailureRetired`.
- An engine ends for the stop of the cluster (the context of `Run`) differently: it stops where it is, nothing is written and nothing is sent back, and it is made again after a restart from its latest checkpoint and its log. A message that waited in its channel is not lost: its sender holds it until it is acked, and sends it again. The cluster stops the router after the engines, so no send is refused for the stop.

## IDs

The orchestrator gives the numbers and keeps the highest of each type in a file, and a recovery cuts what is above it: [ids](ids.md#who-gives-the-numbers). It belongs to the cluster (`Cluster.Orchestrator()`), not to the `Environment` that every engine gets, and it also brings the configuration store into the coordinator engine (`Orchestrator.SyncConfig`).

## Replay and recovery

| Case | What happens |
|---|---|
| a log | the replay makes the engine from its latest checkpoint when it starts from one, else from the checkpoint inside `Init`; `Init` and `Started` are left out of the records the engine is compared with |
| a `Directive` record | put in the router of the replay for the engine to take, as the engine took it |
| the requests and the acknowledgements of a replayed engine | thrown away: the log says which engines were made |
| the log ends with `Retired` | the replay retires the engine at that tick (`Result.Retired`) |
| a handover: an engine with `Init` and nothing after it | kept and run when the log of the engine that asked, as it is kept, holds the response, also when that engine retired after: it writes `Started` at its next tick, with the tick found there. Dropped, with `Retired` written to its log, when the request still waits in the engine that asked (it is published again at its first tick and made again, with another ID), or when its log holds no response (it was gone before it took one) |
| a handover: the other engines | made again from their checkpoints at their limits (`Cluster.AddEngineByCheckpoint`) |
| `Started` lost | the ticks that depend on what the engine did are cut, and the engine is run on from `Init` |
| `Init` lost | the response that depends on it is cut with the tick of the engine that took it, so the request waits and is published again |

## Starting `cmd/world`

`-new-dir DIR` starts a new cluster that writes under `DIR`, which must hold no log: it sends the coordinator engine `CreateEngineCommand` for the world and steps until the world runs. `-replay-dir DIR` cuts the logs under `DIR` to the recovery line, runs each engine on its log, compares it with what was recorded (a divergence is an error), and goes on live, writing after the cut.
