# Configuration engine

`platform/configengine`. The engine that owns the configuration. Every cluster has one (`engine.NewCluster` makes it, first, and `Cluster.ConfigEngine()` returns it); its state is of the last state type (`sim.ConfigStateType`, 255) and its ID is `sim.ConfigStateID`. Where the variables are declared and parsed: `platform/config`.

## What it holds

| Part | Value |
|---|---|
| state | `configengine.State`: the tick, the version, the overrides (variable name and the text of its JSON value) and the snapshot made from them |
| read | `State.Snapshot()` is the only way out: a `config.ReadOnlySnapshot`, which a `Var[T].Get` resolves; the snapshot never changes, so a copy of the state shares it |
| change | `State.ApplyConfigChange(set, unset)`: the version goes up by one when a variable ends with another value, stays when none does, and a wrong value refuses the whole change |
| codecs | `configengine.Register(c)`; `engine.NewCodecs()` makes codecs that have it |

## Changing it

| Step | What happens |
|---|---|
| 1 | a config service makes the command from the document it has: `configengine.NewUpdateConfigCommand(current, document)`; `current` is `cluster.Config()`; no command (`false`) when no variable would change, an error when a value is wrong |
| 2 | `cluster.ConfigEngine().Send(cmd)`: `UpdateConfigCommand` translates to `UpdateConfigAction`, which calls `ApplyConfigChange` |
| 3 | the engine publishes its state at the end of the tick; the other engines read the new version from their next tick |
| after a recovery | the engine is made from its checkpoint and its log like any other, so its version goes on; the service sends the difference between its document and `cluster.Config()` |

## Reading it

| Who | How |
|---|---|
| an engine, at the start of each tick | the copy of the configuration engine that its `StableView` keeps for the tick (`sim.ConfigOf`); the engine that owns the configuration reads its own state |
| an action | `Registry.Config()`: the same copy, so every action of a tick sees one version |
| the event log | the first read in a tick is a `StableRead` of `sim.ConfigStateID` with the tick of the copy, so a replay gives each tick the version it read, and the engine depends on that tick of the configuration engine |
| no copy published | the defaults (`config.Defaults()`), and the `StableRead` has tick 0 |
| from outside the engines | `Cluster.Config()` and `Cluster.Reader()`: the latest copy, with no lock and no record in the log |

## IDs

| What | Number |
|---|---|
| state type | 255 |
| `UpdateConfigCommandID` | 4294967295: the last of the register of commands (32 bits) |
| `UpdateConfigActionID` | 4294967295: the last of the register of actions (32 bits) |

The numbers of the game count up from 1, so they never meet these.
