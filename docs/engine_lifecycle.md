# Making and removing engines

`platform/engine` (`cluster.go`, `engine.go`, `engine_registry.go`, `options.go`, `stepping`), `platform/sim` (`directive.go`, `registry.go`, `codecs.go`, `context.go`), `platform/eventlog` (`Started`, `Directive`, `Retired`), `platform/replay` (`run.go`, `replay.go`, `plan.go`), `server/game` (`codecs.go`, `sim/action` and `sim/command`: `WorldZoneAdd`, `WorldZoneLayerAdd`, `ZoneLayerAnnounceToZone`, `ZoneZoneLayerAdd`). There is no builder of a cluster: the code that runs it adds the world (`Cluster.CreateEngine`), and the world asks for the rest.

## Who is responsible

| What | Who |
|---|---|
| the `StateID` of an engine | **the cluster**: `Cluster.NewStateID(type)` counts the numbers of a type up from 1 and never gives one again. A state does not hold its ID: an action reads it from `Context.Self` |
| the first engine | the configuration engine: `engine.NewCluster` makes it. The first engine of the game is added by the code that runs the cluster: `Cluster.CreateEngine(state, startActions...)` (the actions run once at its first tick) |
| the other engines | the cluster, by the request of an engine (a directive) |
| the state and what the engine starts with | the one that asks: the request holds the state (without an ID) and the actions that run once at its first tick (one that is to run again sets itself again); the zone holds its map, the zonelayer starts with its heal |
| how an engine is made from a state of a type | `engine.EngineRegistry`, which the game fills with one line for each type of state (`server/game/codecs.go`); `Register` also registers the codec of the state |
| which engines a state lists (the zones and zonelayers the world knows) | the state: it takes the answer of the cluster (`sim.DirectiveHandler`) |

## Directives

A directive is a plain struct, not an interface; a `switch` on its `Kind` is in two places: the cluster for the requests, and the state for the answers.

| Struct | Holds |
|---|---|
| `sim.DirectiveRequest` | `ID` (the number that the engine gave it, from 1), `Kind` (`DirectiveCreateEngine`, `DirectiveRemoveEngine`), `Tag` (bytes that the requester writes and reads itself), `State` and `StartActions` (to make), `Target` (to remove) |
| `sim.DirectiveResponse` | `ID`, `Kind`, `Tag` (the one of the request), `Engine` (the ID that was made or removed), `Failure` (empty for success) |

- An action asks with `Registry.RequestCreateEngine(tag, state, startActions...)` or `Registry.RemoveEngine(id, tag)`. Nothing is done while the actions run: the engine numbers the requests when the tick is done, keeps them in its own data (`peerData`: in its checkpoint and its hash), and hands them to its cluster (`Environment.Directives`, the `DirectiveSink` that `NewCluster` sets to the cluster) **before** it sends the messages of the tick, so a message to an engine that the tick makes finds it. With none (a replay) the requests are kept and not handed over.
- The cluster serves a request once, whatever the number of times it is handed over: `Cluster` keeps which engine each `(engine, request ID)` made. It answers the engine that asked with `Engine.HandleDirectiveResponse`.
- The engine takes the answers **first** in its tick (`takeDirectives`), before the messages that came back and before the plan. The answer of a request that is not waiting is dropped with no trace; one that is waiting is written to the log (`eventlog.Directive`, no dependency) and given to the state (`HandleDirective(ctx, registry, response)`), which changes as an action does and has the registry to ask or send. A state that is not a `DirectiveHandler` gets a line of data to fix in the log.
- A request that has no answer is handed over again at the first tick of the engine (also one made again from a checkpoint) and every `directive.resend_every_ticks` ticks (100), so an answer that was lost (a full queue, a cluster made again) comes again. A queue that is full is a warning in the log; an answer for an engine that is gone is lost, and the log holds a line that begins with `datafix:` with what it did not get.

## Making an engine

1. The cluster gives the next number of the type of the state (`state.Type()`), under its lock.
2. `EngineRegistry.Make(env, id, state, startActions, creator, creatorTick, directiveID)` copies nothing it is given (the cluster gives it `state.Clone()`, since the request stays in the data of the engine that asked), makes the engine at the tick of the state, registered with the router, sets the actions it starts with for its first tick (`ScheduleAt(tick+1)`), and **writes the engine as a checkpoint** into the record that begins its log: `eventlog.Started{Creator, CreatorTick, DirectiveID, Snapshot}`. `CreateEngine` does the same with 0, 0 and 0.
3. The engine writes that record first at its tick 1. It is a dependency on the tick of the creator (`Record.Dependency()`), unless no engine asked for it: a recovery that loses that tick cuts the engine to nothing.
4. The engine is added, under the lock, to the list and to the engines that `Run` has not taken yet (`Cluster.addEngineLocked`: every engine goes through it, the configuration engine and the ones added before `Run` too): `Run` is woken and runs it on a goroutine of its own, and the code that steps the engines (`stepping.Step`) runs it from the next step. A cluster runs once: after `Run` returns the router is stopped.
5. The cluster answers with `Engine` set; the world lists the engine when it takes the answer.
6. A failure (a type that the registry does not know, an action that is not one of the state) is the `Failure` of the answer.

A zonelayer announces itself: the action that makes it also sets, for its first tick, `ZoneLayerAnnounceToZone`, which tells its zone its ID (`Context.Self`) and its capacity, since only the zonelayer knows its ID by then.

## Ending an engine

| Cluster is run by | The engine ends |
|---|---|
| `stepping.Step` (a script, a test) | at the end of the step (`Cluster.RetireRequested`), so it runs no tick after the step that asked |
| `Cluster.Run` | on its own goroutine, between two of its ticks (`Engine.RequestRetirement` makes `Engine.Run` call `Engine.Retire` and return), and the cluster drops it |

- `Cluster.RemoveEngine(id)` ends an engine at once when the cluster does not run (between two steps, not from a tick), and asks it to end when `Run` runs it; the configuration engine is never removed. A request from an engine (`DirectiveRemoveEngine`) always asks, and a request for the configuration engine or for an ID that no engine has is dropped.
- `Engine.Retire` writes the last frame of its log and then closes it (`Engine.Close`). `Close` alone writes nothing; `Engine.IsClosed` says that either happened, and `Run` does not run an engine that was closed before it took it.
- Its log gets a last frame, of the tick after the last it ran, that holds `Retired` and no other record: the engine runs no tick of it.
- What it published goes, its queue goes back to its senders (each message once, however many copies wait), a message to it comes back as `DeliveryFailureRetired`, and a message that it held and sent again comes back once and runs its `OnFail` once.
- The tick in which the removal is asked is run to its end. A second request does nothing.
- The list of engines is read and changed under a lock (`CreateEngine`, `Engines`, `Find`, `Get`, `All`, the making and the removal); `Run` takes a copy of it.
- An engine ends for the stop of the cluster (the context of `Run`) differently: nothing is written and it goes on after a restart (`peer.leaveRouter`).

## What the log of an engine holds

- `Started` begins it, for every engine but the configuration engine, and `Retired` ends it when the engine was removed. `Directive` holds the answers that the engine took, in the tick it took them.
- What an engine reads of one that was removed is a copy of a tick it ran, or none: the index refuses a read that names a copy of the tick of the removal or after it.

## Replay and recovery

| Case | What happens |
|---|---|
| a log | the replay makes the engine from its latest checkpoint when it starts from one, else from the checkpoint inside its `Started` record (`Cluster.AddEngineByCheckpoint`, the same as for a checkpoint); the record is not compared, since the engine is made from it; the ID is the one inside it |
| its log has no tick that is kept | nothing to run: it is not made |
| the log ends with `Retired` | the replay removes the engine at that tick (`Result.Retired`) |
| a handover | every engine that the replay has is made again from its checkpoint at its limit (`Cluster.AddEngineByCheckpoint`); one that ended is only refused messages; the cluster goes on giving IDs above the ones that its logs hold (`Plan.LastStateIDs`, `Cluster.RaiseStateIDs`) and knows which request made which engine (`Cluster.RestoreServed`, from the `Started` records), so a request that waits in a checkpoint is not served twice and its answer comes again |
| the frame of a removal was lost | the log does not say it was removed: the engine is made again and goes on |
| the world forgot an engine whose log does not end with `Retired`, or lists one that has no engine | the request that made the engine is handed over again until the answer is taken, so a lost answer comes; an engine whose log is gone is not made again (nothing says what it was), and the game or an admin command puts the world right |

An ID that was given and left no log (its directory never reached the disk) can be given again; one whose log was cut to nothing keeps its directory, so its number is not given again.

## Starting `cmd/world`

`-new-dir DIR` starts a new cluster that writes under `DIR`, which must hold no log; `-replay-dir DIR` cuts the logs under `DIR` to the recovery line, runs each engine on its log, compares it with what was recorded (a divergence is an error), and goes on live, writing after the cut.

## Tests

| Where | Holds |
|---|---|
| `platform/engine/engine_creation_test.go` | the engine is made with the next ID, runs from the next step, starts as the request says and begins its log with its start; the creator takes the answer and writes it to its log; a message of the tick of the request finds the engine; a creator is told when the engine cannot be made; an answer that nothing waits for is dropped; a request that is handed over again is served once and answered once; a request with no answer is handed over again; an answer for an engine that is gone is a line of data to fix; the numbers of a type; `CreateEngine`; an engine made while the cluster runs starts on its goroutine; many goroutines make and remove engines while others run (`-race`); a request that waits in a checkpoint is served by the engine made from it |
| `platform/engine/engine_registry_test.go` | an engine is made from a state of a known type and not of another, an action of another state makes none and registers none, an engine made again from its start writes that record |
| `platform/engine/engine_removal_test.go` | the requests reach the sink once and when the tick is done; no sink, no effect; the step removes the engine and its log ends with `Retired` alone; a message for it comes back as retired; the configuration engine is kept; an unknown ID; an engine that runs on its own ends between two ticks |
| `platform/sim`, `platform/eventlog` | the directives are read back as they were written and keep no view of the bytes; a record of every kind is read back; the first tick of an engine that was made is not safe until the tick of its creator is |
| `server/game/made_engine_test.go` | the zonelayer that the world asks for, its ID and the answer that the world takes; the recovery from a checkpoint and from its start; a handover; a cluster on its goroutines; a creator that lost the tick (cut to nothing, its ID not given again); a request whose answer was lost to a crash is not served again and its answer comes; an engine that cannot be made; a removal lost to a crash; every engine losing its log at several ticks |
| `server/game/removed_engine_test.go` | the recovery of a world in which an engine was removed (by `RemoveEngine`, by a request, by a cluster on its goroutines); a handover does not make it again |
