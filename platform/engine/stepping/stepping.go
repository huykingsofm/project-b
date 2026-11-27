// Package stepping runs the engines of a cluster one tick at a time, one after the other, on the
// goroutine of the caller, in the order they were added: the way to run them for a script or a
// test that needs the same run every time. A cluster that serves clients runs them with
// Cluster.Run, each on a goroutine of its own, and none of them waits for another.
package stepping

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/sim"
)

// engineHandle is what Step needs of an engine of the cluster: to say which engine it is, to run
// one tick, and to retire when it was asked to.
type engineHandle interface {
	StateID() sim.StateID
	Step() error
	RetireIfRequested() bool
}

// Step runs one tick of every engine of the cluster, one after the other. A message that an engine
// sends is in the queue of its receiver at once, so an engine that comes later reads in this step
// what an earlier one sent in it, and an earlier one reads in the next step what a later one sent.
// The configuration that the store holds is sent to the configuration engine first. When the tick
// of an engine fails the cluster stops: the router refuses every message from then on, the engines
// after it do not run, and the error is returned. An engine that an action of this step asked to
// remove is removed when the step ends, and does not run in the next.
func Step(c *engine.Cluster) error {
	c.SyncConfig()
	for _, e := range c.Engines() {
		if err := e.(engineHandle).Step(); err != nil {
			c.Env.Router.Stop()
			return fmt.Errorf("the cluster stopped: %w", err)
		}
	}
	for _, e := range c.Engines() {
		if h := e.(engineHandle); h.RetireIfRequested() {
			c.Forget(h.StateID())
		}
	}
	return nil
}

// Steps runs n steps, and stops at the first that fails.
func Steps(c *engine.Cluster, n int) error {
	for range n {
		if err := Step(c); err != nil {
			return err
		}
	}
	return nil
}
