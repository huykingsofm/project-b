# Making and removing engines

`platform/engine` (`cluster.go`, `engine.go`, `engine_registry.go`, `options.go`, `stepping`), `platform/sim` (`coordinator.go`, `registry.go`, `codecs.go`, `context.go`), `platform/eventlog` (`Started`, `Retired`), `platform/replay` (`run.go`, `replay.go`, `plan.go`), `server/game` (`codecs.go`, `sim/action` and `sim/command`: `WorldZoneAdd`, `WorldZoneLayerAdd`, `ZoneAnnounceToWorld`, `ZoneLayerAnnounceToZone`, `ZoneLayerAnnounceToWorld`, `WorldZoneRegister`, `WorldZoneLayerRegister`, `ZoneZoneLayerAdd`). There is no builder of a cluster: the code that runs it adds the world (`Cluster.CreateEngine`), and the world asks for the rest.

## The coordinator

Every cluster has two engines that no one asks for: the **configuration engine** and the **coordinator** (`sim.CoordinatorState`, `sim.CoordinatorStateID`). The coordinator is an engine like any other: it has a log, takes checkpoints, takes messages and commands, and is replayed. What it does is make and end the other engines.

| What | Who |
|---|---|
| the `StateID` of an engine | **the coordinator**: its state holds the highest number given for each type of state (`CoordinatorState.NextEngineID`), counted up from 1, in its checkpoint and its hash. A state does not hold its ID: an action reads it from `Context.Self` |
| the first engine | the code that runs the cluster: `Cluster.CreateEngine(state, startActions...)` sends the coordinator a `sim.CreateEngineCommand` and returns the ID |
| the other engines | an engine asks the coordinator with a message: `Registry.RequestCreateEngine(onFail, state, startActions...)` sends a `sim.CreateEngineAction` |
| the state and what the engine starts with | the one that asks: the request holds the state (without an ID) and the actions that run once at its first tick (one that is to run again sets itself again); the zone holds its map, the zonelayer starts with its heal |
| how an engine is made from a state of a type | `engine.EngineRegistry`, which the game fills with one line for each type of state (`server/game/codecs.go`); `Register` also registers the codec of the state |
| which engines a state lists (the zones and zonelayers the world knows) | the state: the engine that is made tells it that it is there, with an action it starts with (`ZoneAnnounceToWorld`, `ZoneLayerAnnounceToWorld`), since only that engine knows its ID by then (R28) |

A request is a message, so it has what a message has: a number from its sender, an ack, and a second copy that is sent again is dropped by the coordinator, which keeps the last number it took from each sender (in its checkpoint). Nothing else is needed to serve a request once.

## Making an engine

1. The coordinator takes the message at its next tick. The action `CreateEngineAction` gives the next number of the type of the state (`state.Type()`) and puts the engine in the registry of the tick (`EngineCreation`); nothing else happens while the actions run.
2. When the tick is done, before the messages of the tick are sent, the engine of the coordinator makes the engine: `EngineRegistry.Make(env, id, state, startActions, creator, creatorTick)` makes it at the tick of the state, registered with the router, sets the actions it starts with for its first tick (`ScheduleAt(tick+1)`), and **writes the engine as a checkpoint** into the record that begins its log: `eventlog.Started{Creator, CreatorTick, Snapshot}`, where the creator is the coordinator and the tick is the one it made the engine at.
3. The engine writes that record first at its tick 1. It is a dependency on the tick of the coordinator (`Record.Dependency()`): a recovery that loses that tick cuts the engine to nothing.
4. The coordinator hands the engine to `Environment.Starter` (`engine.EngineStarter`, which `NewCluster` sets to the cluster): the engine is added, under the lock, to the list and to the engines that `Run` has not taken yet (`Cluster.EngineMade`): `Run` is woken and runs it on a goroutine of its own, and the code that steps the engines (`stepping.Step`) runs it from the next step. A cluster that is a replay has no starter, so its coordinator makes nothing. A request that cannot be made (a type that the registry does not know, an action that is not one of the state) makes no engine, and its number is not given again.
5. The engine starts by telling the engine that wants to know it: the world lists a zone or a zonelayer when it takes the message of that engine.

`Cluster.CreateEngine` is the same for the code that runs the cluster, as a command: while the cluster runs on its goroutines it waits for the next tick of the coordinator, and otherwise it takes that tick itself, so the engine is there when the call returns.

## Ending an engine

An action asks with `Registry.RequestRetireEngine(onFail, id)`, which sends a `sim.RetireEngineAction` to the coordinator (the code that runs the cluster sends a `sim.RetireEngineCommand`). The coordinator never ends itself or the configuration engine. It asks the engine to retire (`EngineHandle.RequestRetirement(by)`, where `by` is its own tick), and the engine ends itself:

| Cluster is run by | The engine ends |
|---|---|
| `stepping.Step` (a script, a test) | at the end of the step (`stepping.Step` retires the engine that was asked to and tells the cluster to forget it, `Cluster.Forget`), so it runs no tick after the step that asked |
| `Cluster.Run` | on its own goroutine, between two of its ticks (`Engine.Run` calls `Engine.Retire` and returns), and the cluster drops it |

- `Engine.Retire` writes the last frame of its log and then closes it (`Engine.Close`). The frame, of the tick after the last it ran, holds `Retired{By}` and no other record: `By` is the tick of the coordinator that asked, and the engine depends on it. A recovery that loses that tick loses the end of the engine too, and it goes on. `Close` alone writes nothing; `Engine.IsClosed` says that either happened, and `Run` does not run an engine that was closed before it took it.
- What it published goes, its queue goes back to its senders (each message once, however many copies wait), a message to it comes back as `DeliveryFailureRetired`, and a message that it held and sent again comes back once and runs its `OnFail` once.
- The tick in which the removal is asked is run to its end. A second request does nothing; a request for an ID that no engine has does nothing.
- The list of engines is read and changed under a lock (`CreateEngine`, `Engines`, `Find`, `Get`, `All`, `Forget`); `Run` takes a copy of it.
- An engine ends for the stop of the cluster (the context of `Run`) differently: nothing is written and it goes on after a restart (`peer.leaveRouter`).

## What the log of an engine holds

- `Started` begins it, for every engine but the configuration engine and the coordinator, and `Retired` ends it when the engine was removed.
- What an engine reads of one that was removed is a copy of a tick it ran, or none: the index refuses a read that names a copy of the tick of the removal or after it.

## Replay and recovery

| Case | What happens |
|---|---|
| a log | the replay makes the engine from its latest checkpoint when it starts from one, else from the checkpoint inside its `Started` record (`Cluster.AddEngineByCheckpoint`, the same as for a checkpoint); the record is not compared, since the engine is made from it; the ID is the one inside it |
| the coordinator | made as the configuration engine is, from its checkpoint or new (`engine.Checkpoints`); it replays its log, and its tick makes no engine in a replay |
| its log has no tick that is kept | nothing to run: it is not made |
| the log ends with `Retired` | the replay retires the engine at that tick with the `By` of the record (`Result.Retired`) |
| a handover | every engine that the replay has is made again from its checkpoint at its limit (`Cluster.AddEngineByCheckpoint`); one that ended is only refused messages; the coordinator comes with its checkpoint, so it goes on giving numbers above the ones it gave |
| the frame of a removal was lost | the log does not say it was removed: the engine is made again and goes on, and the message that asked is sent again |
| the coordinator lost the tick that made an engine | the engine depended on it: it is cut to nothing, and the coordinator gives its number again |
| the world lists a zonelayer whose engine has no log | an engine whose log is gone is not made again (nothing says what it was); the game or an admin command puts the world right |

## Starting `cmd/world`

`-new-dir DIR` starts a new cluster that writes under `DIR`, which must hold no log; `-replay-dir DIR` cuts the logs under `DIR` to the recovery line, runs each engine on its log, compares it with what was recorded (a divergence is an error), and goes on live, writing after the cut.

## Tests

| Where | Holds |
|---|---|
| `platform/engine/engine_creation_test.go` | the engine is made with the next ID, runs from the next step, starts as the request says and begins its log with its start, which depends on the tick of the coordinator; an engine that cannot be made is not made and `CreateEngine` says so; a request delivered twice makes one engine; the numbers go on from the checkpoint of the coordinator; the coordinator of a replay makes none; engines made while the cluster runs get different numbers and start on their goroutines (`-race`) |
| `platform/engine/engine_registry_test.go` | an engine is made from a state of a known type and not of another, an action of another state makes none and registers none, an engine made again from its start writes that record |
| `platform/engine/engine_removal_test.go` | an engine asked to end ends when the step ends and its log ends with `Retired` alone, by the tick of the coordinator; a message for it comes back as retired; the configuration engine and the coordinator are kept; an unknown ID; an engine that runs on its own ends between two ticks; many goroutines ask for removals while others run (`-race`) |
| `platform/sim`, `platform/eventlog` | the actions and commands of the coordinator are read back as they were written and keep no view of the bytes; a record of every kind is read back; the first tick of an engine that was made is not safe until the tick of the coordinator is |
| `server/game/made_engine_test.go` | the zonelayer that the world asks for, its ID and the message that makes the world list it; the recovery from a checkpoint and from its start; a handover; a cluster on its goroutines; a coordinator that lost the tick (the engine cut to nothing, its number given again); an engine that cannot be made; a removal lost to a crash; every engine losing its log at several ticks |
| `server/game/removed_engine_test.go` | the recovery of a world in which an engine was removed (by `testkit.RemoveEngine`, by a request, by a cluster on its goroutines); a handover does not make it again |
