package action

import (
	"math/rand/v2"

	"github.com/huykingsofm/project-b/server/internal/config"
)

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

// Context is what an action sees of the world outside its state while it runs: the tick of its
// engine, the random numbers of its engine, and the configuration as it was when the tick began.
// There is no system clock: time is the tick count, so a run can be repeated.
type Context struct {
	Tick uint64
	Rand *rand.Rand
	Cfg  *config.Snapshot
}

// MaxDepth is how many actions deep a chain may go: an action that adds another action to the plan
// gives it a depth one more than its own, and past this the chain is cut. An action that comes from
// a command or a message is at depth 0.
var MaxDepth = config.NewVar("action.max_depth", 4, config.Range(1, 16))

// MaxAttempts is how many times a message that could not be delivered is sent again.
var MaxAttempts = config.NewVar("bounce.max_attempts", 3, config.Range(1, 20))
