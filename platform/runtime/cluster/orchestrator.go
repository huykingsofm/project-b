package cluster

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/atomicfile"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// StateIDMarksFile is the name of the file of the orchestrator, in the directory of the cluster. It
// is a file, not a directory with a dash in its name, so no one reads it as the log of an engine.
const StateIDMarksFile = "state-id-marks"

// stateIDMarksVersion is the version of the data of the file.
const stateIDMarksVersion = 1

// ReadStateIDMarks reads the file of the orchestrator of the cluster in dir, and says false when
// there is none. A file that cannot be read is an error.
func ReadStateIDMarks(dir string) (map[sim.StateType]uint64, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, StateIDMarksFile))
	if errors.Is(err, os.ErrNotExist) {
		return map[sim.StateType]uint64{}, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("the file of the IDs: %w", err)
	}
	r := binary.NewReader(data)
	if version := r.Uvarint(); r.Err() == nil && version != stateIDMarksVersion {
		return nil, false, fmt.Errorf("the file of the IDs is of the data version %d", version)
	}
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, false, fmt.Errorf("the file of the IDs: %w", err)
	}
	if n > uint64(r.Left()) { // a type takes at least two bytes
		return nil, false, fmt.Errorf("the file of the IDs: %w: %d types", binary.ErrShort, n)
	}
	marks := make(map[sim.StateType]uint64, n)
	for range n {
		t := sim.StateType(r.Uvarint())
		if _, twice := marks[t]; twice {
			return nil, false, fmt.Errorf(
				"the file of the IDs: %w: the type %d is twice", binary.ErrBadValue, t,
			)
		}
		marks[t] = r.Uvarint()
	}
	if err := r.Err(); err != nil {
		return nil, false, fmt.Errorf("the file of the IDs: %w", err)
	}
	if r.Left() != 0 {
		return nil, false, fmt.Errorf("the file of the IDs: %d bytes after it", r.Left())
	}
	return marks, true, nil
}

// Orchestrator is the part of the cluster that coordinates it: it gives the IDs of the engines, and
// brings the configuration of the world into the coordinator engine.
//
// The IDs: it holds the highest number that was given for each type of state, stores it in a file
// of its own in the directory of the logs before it gives a number, and gives the IDs to the
// cluster. A number is never given again, whether an engine had it or not, and a recovery does not
// have to read the logs from the start to find the highest.
//
// The configuration: the store holds the configuration of the world outside the game, and the
// orchestrator sends the coordinator engine what changed in it.
//
// Only the cluster calls it, from the periodic task, one goroutine at a time, so it takes no lock.
type Orchestrator struct {
	// the IDs: the file of the marks (empty for an orchestrator that stores nothing), and the
	// highest number given for each type
	file    string
	highest map[sim.StateType]uint64

	// the configuration: the store (nil means none), the configuration that the coordinator engine
	// has or is sent, and the generation of the document that made it
	configStore            *config.Store
	syncedConfig           config.ReadOnlySnapshot
	syncedConfigGeneration uint64
}

// NewOrchestrator makes the orchestrator of the cluster whose logs are in dir, from the file that a
// cluster before it wrote there, and with every number at 0 when there is none. An empty dir makes
// an orchestrator that stores nothing. A file that cannot be read is an error: the numbers it holds
// are not guessed. configStore is the configuration of the world outside the game, nil for none.
func NewOrchestrator(dir string, configStore *config.Store) (*Orchestrator, error) {
	o := &Orchestrator{highest: map[sim.StateType]uint64{}, configStore: configStore}
	if dir == "" {
		return o, nil
	}
	o.file = filepath.Join(dir, StateIDMarksFile)
	marks, _, err := ReadStateIDMarks(dir)
	if err != nil {
		return nil, err
	}
	o.highest = marks
	return o, nil
}

// NextStateID gives the next ID of a type of state. The numbers of a type are counted up from 1 and
// never given again, so an ID names one thing for good, in a log, in a checkpoint and in a message
// that waits. The file is written, and synced, before the ID is returned, so that no engine has an
// ID that the file does not cover.
func (o *Orchestrator) NextStateID(t sim.StateType) (sim.StateID, error) {
	next := o.highest[t] + 1
	if o.file != "" {
		marks := maps.Clone(o.highest)
		marks[t] = next
		if err := atomicfile.Write(o.file, encodeStateIDMarks(marks)); err != nil {
			return 0, fmt.Errorf("the file of the IDs: %w", err)
		}
	}
	o.highest[t] = next
	return sim.NewStateID(t, next), nil
}

// HighestStateIDs is the highest number that was given for each type of state. The map is a copy.
func (o *Orchestrator) HighestStateIDs() map[sim.StateType]uint64 { return maps.Clone(o.highest) }

// ConfigStore is the configuration of the world outside the game, or nil.
func (o *Orchestrator) ConfigStore() *config.Store { return o.configStore }

// ConfigChanged is a channel that has a value when the store took a document; it never has one
// when there is no store.
func (o *Orchestrator) ConfigChanged() <-chan struct{} {
	if o.configStore == nil {
		return nil
	}
	return o.configStore.Changed()
}

// SyncConfig sends the coordinator engine what the store holds, when it is a document that was not
// sent: the command makes the difference from what the engine has, which the orchestrator knows,
// being the only one that sends it. A store that took no document leaves the engine as it is. When
// the engine cannot take the command now, the next call sends it again.
func (o *Orchestrator) SyncConfig(coordinator configReceiver) {
	if o.configStore == nil {
		return
	}
	current := o.configStore.Current()
	if current.Generation == 0 || current.Generation == o.syncedConfigGeneration {
		return
	}
	cmd, changed := sim.NewUpdateConfigCommand(o.syncedConfig, current.Snapshot)
	if changed && !coordinator.SendCommand(cmd) {
		return
	}
	o.syncedConfig, o.syncedConfigGeneration = current.Snapshot, current.Generation
}

// beginConfigSync tells the orchestrator the configuration that the coordinator engine has, which
// it sends the differences from. The cluster calls it once, when it makes the coordinator engine.
func (o *Orchestrator) beginConfigSync(coordinatorConfig config.ReadOnlySnapshot) {
	o.syncedConfig = coordinatorConfig
}

// configReceiver is what the orchestrator needs of the coordinator engine: to be sent a command.
type configReceiver interface {
	SendCommand(c sim.Command[*sim.CoordinatorState]) bool
}

func encodeStateIDMarks(marks map[sim.StateType]uint64) []byte {
	var w binary.Writer
	w.Uvarint(stateIDMarksVersion)
	types := slices.Sorted(maps.Keys(marks))
	w.Uvarint(uint64(len(types)))
	for _, t := range types {
		w.Uvarint(uint64(t))
		w.Uvarint(marks[t])
	}
	return w.B
}
