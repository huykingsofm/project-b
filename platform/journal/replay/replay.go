// Package replay brings a cluster back to a state that its engines had, from what it wrote to its
// directory: the event logs of its engines and their checkpoints. It works out the point of the
// logs that no crash takes back (the recovery line), which is the plan, and runs each engine again
// from its latest checkpoint to its limit on its own log, with other adapters in place of the
// router, the published states and the event log. Every input of an engine comes from the log:
// the commands, the messages it took (in full), the messages of its own that came back and why, and
// which copy of another engine each read found, the copy of the coordinator engine included.
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

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/runtime/cluster"
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/sim"
)

// Result is what a replay found.
type Result struct {
	// Plan is what the replay ran: the limits and the starts.
	Plan *RecoveryPlan
	// Cluster is the cluster that was replayed, each engine at its limit: its states can be read.
	Cluster *cluster.Cluster
	// Ticks is the last tick of each engine that was replayed.
	Ticks map[sim.StateID]uint64
	// Retired is the engines that the replay removed, since their logs end with the record of their
	// removal: the cluster does not hold them.
	Retired []sim.StateID
	// Divergences says, for each engine that did not do what its log says, the first thing that
	// differed. A replay that matches has none.
	Divergences []string
}

// Run runs the cluster that was recorded again, on the logs of the plan, each engine from its start
// to its limit, and reports whether each engine did what its log says. The options must be the ones
// the cluster was made with (the same seed and queue size, and the codecs and the registry of the
// same game); what they set for the event log, the emitter and the checkpoints is not used. It
// changes nothing on the disk.
//
// It returns an error when the replay cannot run: the options are wrong, an engine has a record it
// cannot be given, or the engines wait for each other for ever. A difference between the run and
// the log is not an error but a divergence in the result.
func Run(o engine.Options, p *RecoveryPlan) (*Result, error) {
	run, err := newReplayRun(o, p)
	if err != nil {
		return nil, err
	}
	defer run.close()
	if err := run.play(); err != nil {
		return nil, err
	}
	return &Result{
		Plan: p, Cluster: run.cluster, Ticks: run.ticks(), Retired: run.retired,
		Divergences: run.div.list(),
	}, nil
}

// Recover cuts the logs and the checkpoints under dir to the recovery line, then makes each engine
// of the cluster from its latest checkpoint at or below the line, or from the record that begins
// its log, and runs it on its log to the line, comparing what it does with what it recorded. No log
// may be open for writing. It returns an error when the files cannot be read or cut, or the replay
// cannot run. An engine that did not do what its log says is a *DivergenceError, returned with the
// result, which holds the cluster as it is.
func Recover(dir string, o engine.Options) (*Result, error) {
	if o.Codecs == nil {
		return nil, errors.New("recover: no codecs to read the logs with")
	}
	plan, err := MakeRecoveryPlan(dir, o.Codecs)
	if err != nil {
		return nil, fmt.Errorf("recover: %w", err)
	}
	if err := plan.Cut(); err != nil {
		return nil, fmt.Errorf("recover: %w", err)
	}
	result, err := Run(o, plan)
	if err != nil {
		return nil, fmt.Errorf("recover: %w", err)
	}
	if len(result.Divergences) > 0 {
		return result, &DivergenceError{Divergences: result.Divergences}
	}
	return result, nil
}

// Handover makes the engines of the cluster again in the environment given, each from a checkpoint
// of its replayed engine at its limit, and returns the cluster, which has not run yet: the caller
// runs it, and the engines go on from their own ticks. The environment is the one of a cluster
// that runs (engine.NewEnvironment: its router, event log, published states, emitter, acks and
// checkpoint writer, and the codecs, the registry and the seed of the replay); the event log in it
// writes on after the cut, so no log may be open for writing before. The store is the
// configuration of the world outside the game, which the cluster sends to the coordinator engine.
//
// The replay has an AckBoard, so each replayed engine holds, and its checkpoint carries, the
// messages it sent that no receiver took in the part of its log that was kept. At its first flush
// a live engine lets go of what its receivers acked from their checkpoints and sends the rest
// again: a receiver whose log was cut takes one now, and one that took it drops the copy by its
// number. Each engine that ran a tick publishes its state before its next tick, for the engines
// that read it.
//
// An engine that the replay removed (its log ends with the record of its removal) is not made
// again; messages to it are refused. An engine that a request made and that never ran (its log
// holds Init and nothing after) is made again and run when the engine that asked took the response,
// and ended, with a last frame that says it retired, when the request still waits there: it is made
// again with another ID. The orchestrator of the cluster goes on from the file of the IDs.
//
// A replay in which an engine differs from its log is not handed over. A checkpoint that cannot
// make its engine is an error.
func (r *Result) Handover(env *engine.Environment, store *config.Store) (*cluster.Cluster, error) {
	if len(r.Divergences) > 0 {
		return nil, errors.New("handover: the replay differs from its logs")
	}
	var coordinatorCheckpoint []byte
	if e := r.Cluster.CoordinatorEngine(); e.Tick() > 0 {
		data, err := e.EncodeCheckpoint()
		if err != nil {
			return nil, fmt.Errorf("handover: the checkpoint of the coordinator engine: %w", err)
		}
		coordinatorCheckpoint = data
	}
	orchestrator, err := cluster.NewOrchestrator(r.Plan.dir, store)
	if err != nil {
		return nil, fmt.Errorf("handover: %w", err)
	}
	live, err := cluster.NewCluster(env, orchestrator, coordinatorCheckpoint)
	if err != nil {
		return nil, fmt.Errorf("handover: %w", err)
	}
	inits, err := r.Plan.initRecords()
	if err != nil {
		return nil, fmt.Errorf("handover: %w", err)
	}
	for _, handle := range r.Cluster.Engines() {
		e := handle.(handoverEngineHandle)
		id := e.StateID()
		if id == sim.CoordinatorStateID {
			continue
		}
		init, byRequest := inits[id]
		adoptedAt := uint64(0)
		if byRequest && e.Tick() == 1 {
			// it never ran: it stays when the engine that asked took the response, and not when
			// the request still waits (it is made again, with another ID) or the engine is gone
			keep, tick, err := r.askerTookTheResponse(init)
			if err != nil {
				return nil, fmt.Errorf("handover: the engine %v: %w", id, err)
			}
			if !keep {
				env.EventLog.Append(id, 2, eventlog.Retired{})
				env.EventLog.EndTick(id, 2)
				continue
			}
			adoptedAt = tick
		}
		data, err := e.EncodeCheckpoint()
		if err != nil {
			return nil, fmt.Errorf("handover: the checkpoint of %v: %w", id, err)
		}
		madeAgain, err := live.AddEngineByCheckpoint(data)
		if err != nil {
			return nil, fmt.Errorf("handover: the engine %v: %w", id, err)
		}
		again := madeAgain.(handoverEngineHandle)
		if adoptedAt > 0 {
			again.Adopt(init.Creator, adoptedAt)
		}
		// an engine that has run no tick publishes nothing, as one that was never stopped does: a
		// read that finds no copy is recorded as the tick 0, and a copy at the tick 0 would be
		// recorded the same
		if again.Tick() > 0 {
			again.PublishState()
		}
	}
	for _, id := range r.Retired {
		env.Router.MarkRetired(id)
	}
	if coordinator := live.CoordinatorEngine(); coordinator.Tick() > 0 {
		coordinator.PublishState()
	}
	return live, nil
}

// askerTookTheResponse says whether the engine that asked for an engine that never ran took the
// response to its request, and the tick at which it did: the log of the engine that asked, as it
// is kept, holds the response. It did not when the request still waits in it. An engine that asked
// and then retired took the response when its log holds it, as one that goes on does.
func (r *Result) askerTookTheResponse(init eventlog.Init) (bool, uint64, error) {
	asker, found := r.Cluster.Find(init.Creator)
	if found && asker.(handoverEngineHandle).WaitsForDirectiveResponse(init.DirectiveID) {
		return false, 0, nil
	}
	tick, found, err := r.Plan.directiveResponseTick(
		init.Creator, init.CreatorTick, init.DirectiveID,
	)
	return found, tick, err
}

// DivergenceError is the error of a recovery in which an engine did not do what its log says.
type DivergenceError struct {
	Divergences []string
}

func (e *DivergenceError) Error() string {
	return fmt.Sprintf("the replay differs from the logs in %d engines:\n%s",
		len(e.Divergences), strings.Join(e.Divergences, "\n"))
}

// handoverEngineHandle is what Handover needs of an engine of a cluster: to say which engine it
// is and where it is, to take its checkpoint and, once made again, to publish its state.
type handoverEngineHandle interface {
	StateID() sim.StateID
	Tick() uint64
	EncodeCheckpoint() ([]byte, error)
	PublishState()
	WaitsForDirectiveResponse(request uint64) bool
	Adopt(creator sim.StateID, responseTick uint64)
}

var _ handoverEngineHandle = (*engine.Engine[*sim.CoordinatorState])(nil)
