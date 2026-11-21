# R3. Where shared code lives (hard)

Back to the [index](index.md).

- **Use an external library when one exists.** Before writing a generic container or mechanism, look for a maintained external library that does the job and use it. Only when none exists is it implemented in `shared`.
- `server/internal/shared` holds what no external library provides, and is treated as a set of external libraries that we happen to write ourselves. Each one is **its own package**, has its own tests, and **imports nothing of the project** except other packages of `shared`. They carry no business of the game: no word and no concept of the game. A package of `shared` must read like a library that could move to a repository of its own without a change.
- **The plan, the scheduler and the cron are game logic, not shared code.** The plan (the actions of a tick sorted by priority, with a cursor and indexes by kind, target and engine), the scheduler (actions that run at a tick) and the cron (actions that run every N ticks) are how the game runs actions, so they live in `server/internal/services/game/sim/action` (the plan in `action.go`, the scheduler in `scheduler.go` and the cron in `cron.go`), never in `shared`.
- `server/internal/services/game/sim` holds the whole game system: state, actions, commands, plan, scheduler, cron. It has no interaction between engines and none with the outside.
- `server/internal/services/game/engine` runs the engines and connects them (engines, router, registry of published states, graph, replay log). It does not know what the actions do.
- When in doubt whether something is "shared", ask: does it mention or assume anything of the game (an action, a tick, an entity, a priority)? If it does, it belongs in `sim`.
