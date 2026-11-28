// Package stepping runs the engines of a cluster one tick at a time, one after the other, on the
// goroutine of the caller, in the order they were added: the way to run them for a script or a
// test that needs the same run every time. A cluster that serves clients runs them with
// Cluster.Run, each on a goroutine of its own, and none of them waits for another.
package stepping

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/runtime/cluster"
	"github.com/huykingsofm/project-b/platform/sim"
)

// Step runs one tick of every engine of the cluster, one after the other. A message that an engine
// sends is in the queue of its receiver at once, so an engine that comes later reads in this step
// what an earlier one sent in it, and an earlier one reads in the next step what a later one sent.
// The configuration that the store holds is sent to the coordinator engine first. When the tick
// of an engine fails the cluster stops: the router refuses every message from then on, the engines
// after it do not run, and the error is returned. An engine that an action of this step asked to
// retire is retired when the step ends, and does not run in the next.
func Step(c *cluster.Cluster) error {
	c.RunPeriodicTasks()
	for _, e := range c.Engines() {
		if err := e.(engineHandle).Step(); err != nil {
			c.Env.Router.Stop()
			return fmt.Errorf("the cluster stopped: %w", err)
		}
	}
	retireRequested(c)
	return nil
}

// RetireEngine ends the engine with the ID now, between two steps, and drops it from the cluster:
// the engine is asked to retire (Cluster.RequestEngineRetirement) and retires at once, since no
// goroutine of its own runs it. It reports whether the cluster had the engine. It is not for a
// cluster that Run runs: there the engine retires on its own goroutine.
func RetireEngine(c *cluster.Cluster, id sim.StateID) bool {
	if !c.RequestEngineRetirement(id) {
		return false
	}
	retireRequested(c)
	return true
}

// Steps runs n steps, and stops at the first that fails.
func Steps(c *cluster.Cluster, n int) error {
	for range n {
		if err := Step(c); err != nil {
			return err
		}
	}
	return nil
}

// engineHandle is what Step needs of an engine of the cluster: to say which engine it is, to run
// one tick, and to retire when it was asked to.
type engineHandle interface {
	StateID() sim.StateID
	Step() error
	RetireIfRequested() bool
}

// retireRequested retires the engines that were asked to retire and drops them from the cluster.
func retireRequested(c *cluster.Cluster) {
	for _, e := range c.Engines() {
		if h := e.(engineHandle); h.RetireIfRequested() {
			c.Forget(h.StateID())
		}
	}
}
