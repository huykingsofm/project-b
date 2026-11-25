// Package recovery brings a cluster back from what it wrote to its directory: the replay logs of
// its engines and their checkpoints. It works out the point of the logs that no crash takes back
// (the recovery line), cuts the logs and the checkpoints to it, and runs each engine again from
// its latest checkpoint to the end of its log. The engines that were made are the ones that go on
// from there.
package recovery

import (
	"errors"
	"fmt"

	"github.com/huykingsofm/project-b/platform/checkpoint"
	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/replay/runner"
	"github.com/huykingsofm/project-b/platform/sim"
)

// Options are what a recovery needs that the files do not hold.
type Options struct {
	// ConfigVersions says what each version of the configuration was made of (see
	// runner.Options).
	ConfigVersions map[uint64]map[string]string
}

// Result is what a recovery did.
type Result struct {
	// Limits is the last tick of each engine that was kept: the recovery line.
	Limits map[sim.StateID]uint64
	// Starts is the tick each engine was started at: the tick of its checkpoint, or 0.
	Starts map[sim.StateID]uint64
	// Replay is what the replay of the logs from the starts found: the cluster at the limits, and
	// the divergences.
	Replay *runner.Result
}

// Recover cuts the logs and the checkpoints under dir to the recovery line, then makes the cluster
// that b describes from the latest checkpoint of each engine that is at or below the line, and runs
// each engine on its log to the line, comparing what it does with what it recorded. No log may be
// open for writing. It returns an error when the files cannot be read or cut, or the replay cannot
// run; an engine that did not do what its log says is a divergence in the result.
func Recover(dir string, b runner.Builder, opts Options) (*Result, error) {
	codecs := b.Codecs()
	if codecs == nil {
		return nil, errors.New("recovery: the builder cannot make its codecs")
	}
	limits, err := CutToRecoveryLine(dir, codecs)
	if err != nil {
		return nil, fmt.Errorf("recovery: %w", err)
	}
	starts, err := chooseStarts(dir, codecs, limits)
	if err != nil {
		return nil, fmt.Errorf("recovery: %w", err)
	}
	logs, err := runner.FileLogs(dir, codecs)
	if err != nil {
		return nil, fmt.Errorf("recovery: %w", err)
	}
	replayed, err := runner.Run(b, logs, runner.Options{
		ConfigVersions: opts.ConfigVersions,
		Starts:         starts,
		Checkpoints:    CheckpointFiles(dir, starts),
	})
	if err != nil {
		return nil, fmt.Errorf("recovery: %w", err)
	}
	return &Result{Limits: limits, Starts: starts, Replay: replayed}, nil
}

// CheckpointFiles is the engine.CheckpointSource of the checkpoint files under dir: for each engine
// that starts above tick 0 in starts, the file of its checkpoint at that tick. An engine that is
// not in starts, or is at 0, has none and is made new.
func CheckpointFiles(dir string, starts map[sim.StateID]uint64) engine.CheckpointSource {
	return fileCheckpoints{dir: dir, starts: starts}
}

// fileCheckpoints is the source that CheckpointFiles makes.
type fileCheckpoints struct {
	dir    string
	starts map[sim.StateID]uint64
}

func (f fileCheckpoints) Checkpoint(id sim.StateID) ([]byte, bool, error) {
	start := f.starts[id]
	if start == 0 {
		return nil, false, nil
	}
	data, err := checkpoint.Read(f.dir, id, start)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}
