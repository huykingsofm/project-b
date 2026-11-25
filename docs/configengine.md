# Configuration engine

`platform/sim/config.go`. The engine that owns the configuration. Every cluster has one (`engine.NewCluster` makes it, first, and `Cluster.ConfigEngine()` returns it); its state is of the last state type (`sim.ConfigStateType`, 255) and its ID is `sim.ConfigStateID`. Where the variables are declared and parsed: `platform/config`.

## What it holds

| Part | Value |
|---|---|
| state | `sim.ConfigState`: the tick and the snapshot, which holds the value of every variable and the text of its JSON value for each one that was set (`Snapshot.Texts()`, in the order of the names); a checkpoint writes the tick and `Snapshot.Encode`, and reads them back with `config.DecodeSnapshot` |
| read | `State.Snapshot()` is the only way out: a `config.ReadOnlySnapshot`, which a `Var[T].Get` resolves; the snapshot never changes, so a copy of the state shares it |
| change | `ConfigState.ApplyConfigChange(set, unset)`, which is `Snapshot.Apply`: the snapshot is a new one when a variable ends with another value, stays when none does, and a wrong value refuses the whole change |
| codecs | `sim.NewCodecs()` makes codecs that have it, as every cluster needs them |

## Changing it

| Step | What happens |
|---|---|
| 1 | whoever has the configuration of the world outside the game (a file, a service) gives it to the `config.Store` of the cluster: `store.Apply(document)`, the text of the JSON value of each variable that is not at its default, by name. A wrong value is refused there and changes nothing |
| 2 | the cluster sends it: at the start of every `Step`, and in `Run` when the store signals (`Changed()`) or at every period, it makes the command from the document of the store and what it sent last, `sim.NewUpdateConfigCommand`, and `ConfigEngine().Send(cmd)`. No command when no variable would change; when the engine cannot take it, the next call sends it again. A store that took no document leaves the engine as it is |
| 3 | `UpdateConfigCommand` translates to `UpdateConfigAction`, which calls `ApplyConfigChange`; the command is in the log of the configuration engine |
| 4 | the engine publishes its state at the end of the tick; the other engines read the new copy from their next tick |
| after a recovery | the engine is made from its checkpoint and its log like any other, so its configuration goes on; the cluster starts from what the engine holds, and the first call sends the difference with the store |

## Reading it

| Who | How |
|---|---|
| an engine, at the start of each tick | the copy of the configuration engine that its `StableView` keeps for the tick (the engine reads it with `Registry.Config()`, and its `peer` for the resend of messages); the engine that owns the configuration reads the copy of its last tick like any other, and the log records that read |
| an action | `Registry.Config()`: the same copy, so every action of a tick sees one configuration |
| the event log | the first read in a tick is a `StableRead` of `sim.ConfigStateID` with the tick of the copy, so a replay gives each tick the copy it read, and the engine depends on that tick of the configuration engine |
| no copy published | the defaults (`config.Defaults()`), and the `StableRead` has tick 0 |
| from outside the engines | `Cluster.Reader()`: the latest copy of the configuration engine, with no lock and no record in the log; `Cluster.ConfigStore().Snapshot()` is the configuration of the world outside the game, which the `LineCacheWriter` reads for its period |
