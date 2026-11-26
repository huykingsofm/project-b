# Removing an engine while the game runs

`platform/engine` (`control.go`, `cluster.go`, `engine.go`), `platform/sim/registry.go`, `platform/replay` (`handover.go`, `replay.go`), `server/game/cluster_builder.go`. What the log writes for an engine that was removed: [eventlog](eventlog.md#writing); how a replay runs it: [replay](replay.md).

## Asking

| Call | What it does |
|---|---|
| `Registry.RequestEngineRemoval(id)` | an action asks for the engine with the ID to be removed; nothing is done while the actions run |
| `Env.Control` (`engine.ClusterController`) | where the engine hands the requests of the tick to, when the tick is done (`RemoveEngine(id)`); with none the requests are ignored, as in a replay |
| `engine.NewClusterControl()` | the control of a cluster that runs; `NewCluster` attaches the cluster to the one its environment holds; `ClusterBuilder.NewEnv` makes the environment with one |
| `Cluster.RemoveEngine(id)` | asks the cluster to remove the engine, without waiting; a request for the configuration engine or for an ID that no engine has is dropped |
| `Cluster.Remove(id)` | removes the engine and says whether the cluster had it; the configuration engine is never removed; while the cluster runs it only asks, as `RemoveEngine` does |

- The state that lists the engine (the world, for a zonelayer) is changed by the action, as for any change of a state (R28); the request only ends the engine. What the game moves out of the engine first (its characters and items) is the game's.

## Who ends the engine and when

| Cluster is run by | The engine ends |
|---|---|
| `Cluster.Step` | at the end of the step: `Cluster.Step` removes what was asked while it ran, so the engine runs no tick after the step that asked, and is not in the next |
| `Cluster.Run` | on its own goroutine, between two of its ticks: `Engine.RequestRemoval` makes `Engine.Run` call `Engine.Remove` and return, and the cluster drops the engine from its list |

- The tick in which the removal is asked is run to its end. The removal is then the frame of the next tick of the engine, which holds `Retired` and no other record, and the engine runs no tick of it.
- A second request, and one for an engine that has ended, do nothing.
- The list of the engines of a cluster is read and changed under a lock (`Add`, `Engines`, `Find`, `Get`, `All`, the removal); `Step` and `Run` take a copy of it.

## What others see

| Who | What |
|---|---|
| a sender of a message to the engine | `DeliveryFailureRetired` from then on; a message it held and sent again comes back once and its `OnFail` runs once |
| what waits in the channel of the engine | goes back to its senders, once for each message however many copies of it wait |
| an engine that reads it | a copy of a tick it ran, or none: what it published goes when it is removed, and it publishes nothing at the tick of its removal |
| its log and checkpoints | not deleted; the log ends with `Retired` |

## After a crash

| Case | What a recovery and a handover do |
|---|---|
| the log of the engine ends with `Retired` | the replay removes it at that tick (`Result.Retired`), and the handover does not make it again |
| the frame of its removal was lost | the log does not say it was removed: the engine is made again from its latest checkpoint and goes on; the world, which has not forgotten it, still lists it |
| the world forgot the engine in a tick that its log keeps and the engine's log does not end with `Retired` | the builder says so (`replay.OrphanFinder`; `ClusterBuilder.Orphans` is the engines of a zone or a zonelayer that the world no longer lists) and the handover removes the engine, which writes `Retired` to its log |

## Tests

| Where | Holds |
|---|---|
| `platform/engine/engine_removal_test.go` | the request reaches the control once and when the tick is done; no control, no effect; the step removes the engine and its log ends with `Retired` alone; a message for it comes back as retired; the configuration engine is kept; a request for an unknown ID; an engine that runs on its own ends between two ticks; many goroutines ask while others run (`-race`) |
| `server/game/removed_engine_test.go` | the recovery of a world in which an engine was asked to be removed, and of one that ran on its own goroutines and removed one; a handover that removes the engine that the world forgot, and one after a crash that lost the frame of the removal; `ClusterBuilder.Orphans` |
