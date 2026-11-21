# Current state of project-b

## What exists (with tests)

**Simulation (`server/internal/services/game/sim`)**
- Three scopes: world, zone, zonelayer. Each scope has its own state, events, plan, actions and registry.
- A state is only data, changed by its own methods. It has two regions: the data (a `Clone` is published) and the events (not cloned, not published). Each kind of event has an `ID` registered in the `events.go` of its scope.
- The zonelayer state links characters and bodies with `bodies` (`shared/bimap`). Cooldowns are keyed by type (`CooldownUntil[Key]`).
- 11 actions: zonelayer (Move, NormalAttack, Damage, Shield, Regen, ChatSend, Enter, ExitTo), zone (CharacterEnter, ChatSend), world (ChatPrivateSend). Each action is documented in `docs/action/`.
- The plan has priority, a running pointer, an index by ID and by type (`FindType`), a depth limit on chains of actions, a scheduler and a cron, and messages with `OnFail`.
- Leaving a zone is an optimistic exit (only two actions: `ExitTo` and `Enter`).
- `tilemap` (map, exits, spawns).

**Engine (`server/internal/services/game/engine`)**
- One engine per scope, each on its own goroutine; `ClusterBuilder` (`NewBuilder`, `With...`, `Build`) builds the cluster.
- `Router` carries messages between engines with `OutgoingChannel`; `cell` holds the `IncomingChannel`, the config, the replay log and the bouncer.
- `StableStateRegistry` holds the latest state of every engine; each engine reads through a `StableStateView` (fixed within a tick).
- Three `Emitter`s (world, zone, zonelayer) send events out; `ReplayLog` records the input from outside and the hash of the state (sparse, offset per engine).
- `PlanPool` for each scope.

**Shared (`server/internal/shared`) and configuration**
- `orderedmap`, `table`, `registry`, `bimap`, `channels` (`Drain`); `config` (`Var[T]`, snapshot, a whole set is rejected when one value is wrong).

**Rules and documents**: `plan/rule/` (R0 to R29), `docs/action/`, `README.md`, `Makefile` (`make ci`).

## What does not exist

Grid and sight, observe (projecting events per viewer), loading state, characters with stats, combat formulas, effects, storage, network and accounts, client, measuring tools, content (monsters, items, quests, sects), a real replay runner.

## Temporary values

- The damage of normal attack (`1`) and regen (`1`): to be replaced by formulas in phase 4.
