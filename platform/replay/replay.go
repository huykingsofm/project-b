// Package replay brings a cluster back to a state that its engines had, from what it wrote to its
// directory: the event logs of its engines and their checkpoints. It works out the point of the
// logs that no crash takes back (the recovery line), which is the plan, and runs each engine again
// from its latest checkpoint to its limit on its own log, with other adapters in place of the
// router, the published states and the event log. Every input of an engine comes from the log:
// the commands, the messages it took (in full), the messages of its own that came back and why, and
// which copy of another engine each read found, the copy of the configuration engine included.
// What the engines send is made again, compared with what the receivers recorded, and then not
// delivered: the receivers take what their own logs say. What the engines do is compared with what
// the logs say; a difference is a divergence.
//
// The engines run one after another on the goroutine that calls Run. An engine runs its next tick
// when the copies that tick read exist, that is, when the engines they came from have run up to the
// tick of the copy.
package replay

import (
	"errors"
	"fmt"
	"strings"

	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/sim"
)

// Builder is what a replay needs of the game that was recorded: the settings of its cluster, and
// how to build the cluster in the environment that the replay provides, with the codecs that read
// back what was written.
type Builder interface {
	Seed() uint64
	QueueSize() int
	Codecs() *sim.Codecs
	// BuildIn makes each engine from the checkpoint that the source has for it, and new when it has
	// none.
	BuildIn(env *engine.Env, checkpoints engine.CheckpointSource) (*engine.Cluster, error)
}

// Result is what a replay found.
type Result struct {
	// Plan is what the replay ran: the limits and the starts.
	Plan *Plan
	// Cluster is the cluster that was replayed, each engine at its limit: its states can be read.
	Cluster *engine.Cluster
	// Ticks is the last tick of each engine that was replayed.
	Ticks map[sim.StateID]uint64
	// SentMessages is, for each engine, the messages it sent from its start to its limit that did
	// not come back, in the order of their numbers: what the engines of a replay do not hold, and
	// what the engines that go live send again (see Handover).
	SentMessages map[sim.StateID][]sim.Message
	// Divergences says, for each engine that did not do what its log says, the first thing that
	// differed. A replay that matches has none.
	Divergences []string
}

// DivergenceError is the error of a recovery in which an engine did not do what its log says.
type DivergenceError struct {
	Divergences []string
}

func (e *DivergenceError) Error() string {
	return fmt.Sprintf("the replay differs from the logs in %d engines:\n%s",
		len(e.Divergences), strings.Join(e.Divergences, "\n"))
}

// Run runs the cluster that b describes again, on the logs of the plan, each engine from its start
// to its limit, and reports whether each engine did what its log says. The builder must describe
// the cluster that was recorded (the same engines, states and seed); what it sets for the event
// log and the emitter is not used. It changes nothing on the disk.
//
// It returns an error when the replay cannot run: the builder is wrong, an engine has a record it
// cannot be given, or the engines wait for each other for ever. A difference between the run and
// the log is not an error but a divergence in the result.
func Run(b Builder, p *Plan) (*Result, error) {
	run, err := newReplayRun(b, p)
	if err != nil {
		return nil, err
	}
	defer run.close()
	if err := run.play(); err != nil {
		return nil, err
	}
	return &Result{
		Plan: p, Cluster: run.cluster, Ticks: run.ticks(), SentMessages: run.sentMessages(),
		Divergences: run.div.list(),
	}, nil
}

// Recover cuts the logs and the checkpoints under dir to the recovery line, then makes the cluster
// that b describes from the latest checkpoint of each engine that is at or below the line, and
// runs each engine on its log to the line, comparing what it does with what it recorded. No log
// may be open for writing. It returns an error when the files cannot be read or cut, or the replay
// cannot run. An engine that did not do what its log says is a *DivergenceError, returned with the
// result, which holds the cluster as it is.
func Recover(dir string, b Builder) (*Result, error) {
	codecs := b.Codecs()
	if codecs == nil {
		return nil, errors.New("recover: the builder cannot make its codecs")
	}
	plan, err := MakePlan(dir, codecs)
	if err != nil {
		return nil, fmt.Errorf("recover: %w", err)
	}
	if err := plan.Cut(); err != nil {
		return nil, fmt.Errorf("recover: %w", err)
	}
	result, err := Run(b, plan)
	if err != nil {
		return nil, fmt.Errorf("recover: %w", err)
	}
	if len(result.Divergences) > 0 {
		return result, &DivergenceError{Divergences: result.Divergences}
	}
	return result, nil
}
