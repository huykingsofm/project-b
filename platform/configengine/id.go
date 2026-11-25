package configengine

import (
	"math"

	"github.com/huykingsofm/project-b/platform/sim"
)

// The IDs of the command and the action of the configuration engine: the last of each register,
// as its state is the last of the types, so the game's own numbers, which count up from 1, never
// meet them.
const (
	UpdateConfigCommandID sim.CommandID = math.MaxUint32
	UpdateConfigActionID  sim.ActionID  = math.MaxUint32
)
