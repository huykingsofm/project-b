// Package runner runs the engines of a cluster again, the same engines with the same code for a
// tick (Step), with other adapters in place of the router, the published states, the
// configuration and the replay log. Every input of an engine comes from the log: the commands, the
// messages it took (in full), the messages of its own that came back and why, the version of the
// configuration, and which copy of another engine each read found. What the engines send is made
// again, compared with what the receivers recorded, and then not delivered: the receivers take what
// their own logs say.
//
// The engines run one after another on the goroutine that calls Run. An engine runs its next tick
// when the copies that tick read exist, that is, when the engines they came from have run up to the
// tick of the copy.
package runner

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/engine"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
)

// Options are what a replay needs that the records do not hold.
type Options struct {
	// ConfigVersions says what each version of the configuration was made of: the overrides that
	// were given to config.Store.Apply to make it. Version 0 is the defaults.
	ConfigVersions map[uint64]map[string]string
}

// Result is what a replay found.
type Result struct {
	// Cluster is the cluster that was replayed, each engine at the last tick that its log, or a
	// read of another engine, needs: its states can be read.
	Cluster *engine.Cluster
	// Ticks is the last tick of each engine that was replayed.
	Ticks map[message.Address]uint64
	// Divergences says, for each engine that did not do what its log says, the first thing that
	// differed. A replay that matches has none.
	Divergences []string
}

// Run runs the cluster that b describes again from tick 0, on the records in logs, and
// reports whether each engine did what its log says. The builder must describe the cluster that was
// recorded (the same world, zones, maps, zonelayers and seed); what it sets for the replay log, the
// emitters and the configuration is not used.
//
// It returns an error when the replay cannot run: the builder is wrong, an engine has a record it
// cannot be given, or the engines wait for each other for ever. A difference between the run and
// the log is not an error but a divergence in the result.
func Run(
	b *engine.ClusterBuilder, logs Logs, opts Options,
) (*Result, error) {
	run, err := newReplayRun(b, logs, opts)
	if err != nil {
		return nil, err
	}
	if err := run.play(); err != nil {
		return nil, err
	}
	run.finish()
	return &Result{Cluster: run.cluster, Ticks: run.cur, Divergences: run.div.list()}, nil
}
