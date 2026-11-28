# What the platform logs when something is not as expected

`platform/runtime/router` and `platform/runtime/engine` (`tick_times.go`). Nothing is given to a monitoring system: the router and the engines write lines with `shared/logging` ([logging](logging.md)), and what reads them is outside. Nothing is counted for this: a place where something unexpected happens logs it at once, with where it came from and where it was going. A counter is added to such a place when something needs one.

## Messages

| What happened | Where | The warning |
|---|---|---|
| a message found the channel of its receiver full (`DeliveryFailureFull`); its sender handles it by its `OnFail` | `Router.Deliver` | `message refused: the channel of its receiver is full`, with `from` and `dest` |
| a message came back to a sender that is gone | `Router.Return` | `returned sim lost: its sender is gone`, with `from`, `dest` and `reason` |
| a message came back to a sender whose back channel is full | `Router.Return` | `returned sim lost: the back channel of its sender is full`, with `from`, `dest` and `reason` |

- These are not expected, so each is logged each time: a log that fills with them says that a receiver is too slow or a queue too small, which is to be solved, not to be hidden by a count. A message to an engine that is retired or not started yet is an ordinary refusal and is not logged.
- A refused message is not lost: its sender holds it until it is acked and sends it again. A message that is lost is one that its sender cannot take back.

## Time of the ticks

| What | Value |
|---|---|
| measured | the time of each tick of an engine, from the start of `Step` to its end |
| classes | the class `i` holds the ticks that took less than 2^i microseconds and at least half of that; the last holds the ticks of 65 ms and more |
| logged | every `engine.log_tick_times_every_ticks` (6000) ticks, an information line with the engine, the ticks counted, `p50` and `p99` (the upper bound of the class that holds them), the longest and how many ticks fell in each class; then the count starts again |

- This is a measure, not a problem, so it is a summary of many ticks.
- The engine reads the clock to measure its ticks and to nothing else: what it does does not depend on it, so a replay is not changed.

## Not here

- The number of players and the capacity of a zonelayer are of the game, not of a platform that runs any state; they come with the game.
- No metrics format (Prometheus or other): the lines are the only output.
