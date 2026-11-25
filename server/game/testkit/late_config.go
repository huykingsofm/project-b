package testkit

import (
	"sync/atomic"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/server/game"
)

// LateConfig is the configuration of a cluster that is built after what asks for it, such as the
// observer of a replay log, which the builder is given first. It is the defaults until Set says
// which cluster it is.
type LateConfig struct{ cluster atomic.Pointer[game.Cluster] }

// Set says the cluster whose configuration it is.
func (l *LateConfig) Set(c *game.Cluster) { l.cluster.Store(c) }

// Config is the configuration of the cluster, or the defaults before there is one.
func (l *LateConfig) Config() config.ReadOnlySnapshot {
	if c := l.cluster.Load(); c != nil {
		return c.Config()
	}
	return config.Defaults()
}
