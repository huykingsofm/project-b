# R5. Every pattern has at least one exemplar, and the owner decides the rest (hard)

For every pattern the design relies on (the plan index, the publish of state, the scheduler, a `table.Table` with a column, a standing action, a gate, dispatch by kind, bounce), build at least one working exemplar with its tests and a benchmark, even if no feature uses it yet. The exemplar shows later what the pattern looks like and what using it requires, so that design and code are settled before the first real use and not at the moment it is needed. An exemplar comes with a short note: what a user of the pattern must write, and what it costs. This is not premature optimization: the pattern is part of the design ([R4](r04-essentials-built-now.md)), and only an opt-in speedup waits. Exemplars today: `ZoneLayerCharacterShieldGrantAction` (a standing action that finds other actions in the plan), `ZoneLayerCharacterMoveAction` (an action that sets itself again, through a cooldown, and can be cancelled), `ZoneLayerCharacterRegenAction` (a cron), and `ZoneLayerCharacterExitToAction` (an optimistic action that reads what other scopes published and puts a character back when a message cannot be delivered).

## The owner decides state methods and actions

A method of a state, an action, and the mechanism of either are the owner's to define. Nothing of these is added on the author's own initiative: not as a helper, not as an example "for later", not because it will probably be needed.

When something is needed, say what it is and ask how it should work; the owner decides, and only then is it written. This holds for every new method of a state, every new action, and every new mechanism behind them (a way to wait, a way to cancel, a way to keep a cooldown).

What is in the code today stays as the example of its pattern, and the owner looks at each one in turn. The aim is at least one example of each pattern, and no more than that.

A design that the owner has approved (the plan, a proposed design) is the owner's decision: its methods and actions are written without asking again for each.
