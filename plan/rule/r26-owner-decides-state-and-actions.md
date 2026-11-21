# R26. The owner decides state methods and actions (hard)

A method of a state, an action, and the mechanism of either are the owner's to define. Nothing of these is added on the author's own initiative: not as a helper, not as an example "for later", not because it will probably be needed.

When something is needed, say what it is and ask how it should work; the owner decides, and only then is it written. This holds for every new method of a state, every new action, and every new mechanism behind them (a way to wait, a way to cancel, a way to keep a cooldown).

What is in the code today stays as the example of its pattern ([R6](r06-pattern-exemplar.md)), and the owner looks at each one in turn. The aim is at least one example of each pattern, and no more than that.
