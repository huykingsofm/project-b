package runner

import (
	"github.com/huykingsofm/project-b/server/internal/config"
)

// replayConfig is the configuration of a replay: the snapshot of the version that the engine about
// to run read.
type replayConfig struct{ snap *config.Snapshot }

func (c *replayConfig) Snapshot() *config.Snapshot { return c.snap }
