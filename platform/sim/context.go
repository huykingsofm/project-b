package sim

import (
	"math/rand/v2"

	"github.com/huykingsofm/project-b/platform/config"
)

// Context is what an action sees of the world outside its state while it runs: the tick of its
// engine and the random numbers of its engine. The configuration is read from the registry
// (Registry.Config). There is no system clock: time is the tick count, so a run can be repeated.
type Context struct {
	Tick uint64
	Rand *rand.Rand
}

// MaxDepth is how many actions deep a chain may go: an action that adds another action to the plan
// gives it a depth one more than its own, and past this the chain is cut. An action that comes from
// a command or a message is at depth 0.
var MaxDepth = config.NewVar("action.max_depth", 4, config.Range(1, 16))

// MaxAttempts is how many times a message that could not be delivered is sent again.
var MaxAttempts = config.NewVar("bounce.max_attempts", 3, config.Range(1, 20))
