# Counts and times in the log

`platform/runtime/router` (`Router.Refused`, `Router.Dropped`), `platform/runtime/cluster` (`Cluster.logMessageCounts`) and `platform/runtime/engine` (`tick_times.go`). Nothing is given to a monitoring system: the cluster and the engines write lines with `shared/logging` ([logging](logging.md)), and what reads them is outside.

## Messages

| Count | Where | What it counts | Logged again when it has grown by |
|---|---|---|---|
| refused | `Router.Refused()` | the messages that found the channel of their receiver full (`DeliveryFailureFull`); their senders handled each by its `OnFail` | `router.log_every_refused_messages` (100) |
| lost | `Router.Dropped()` | the messages that came back to a sender that was gone, or whose back channel was full | `router.log_every_lost_messages` (1) |

- The periodic task of the cluster (`RunPeriodicTasks`) reads the two counts and logs a warning when one has grown by its threshold since it was logged: the total and how many are new. A router that has no such counts says nothing.
- A message that is lost is logged at once by the router as well, with where it came from and where it was going.
- The thresholds are variables of the configuration, so they change while the cluster runs.

## Time of the ticks

| What | Value |
|---|---|
| measured | the time of each tick of an engine, from the start of `Step` to its end |
| classes | the class `i` holds the ticks that took less than 2^i microseconds and at least half of that; the last holds the ticks of 65 ms and more |
| logged | every `engine.log_tick_times_every_ticks` (6000) ticks, an information line with the engine, the ticks counted, `p50` and `p99` (the upper bound of the class that holds them), the longest and how many ticks fell in each class; then the count starts again |

- The engine reads the clock to measure its ticks and to nothing else: what it does does not depend on it, so a replay is not changed.

## Not here

- The number of players and the capacity of a zonelayer are of the game, not of a platform that runs any state; they come with the game.
- No metrics format (Prometheus or other): the lines are the only output.
