package action

// The priorities of the actions, in the order they run: a smaller number runs first. They are
// bands, so a new kind of action says in which band it runs and the rest follows:
//
//   - system: what an engine does whatever anyone asked for (healing over time, reading what
//     others published);
//   - intent: what an engine asked for, before anything is worked out (a move, an attack);
//   - modifier: standing actions that change the actions behind them (a shield changes the damage
//     that is about to land);
//   - resolve: the actions that change the state as a result (damage, a message being applied).
const (
	PrioritySystem   int16 = -200
	PriorityIntent   int16 = -100
	PriorityModifier int16 = -50
	PriorityResolve  int16 = 0
)
