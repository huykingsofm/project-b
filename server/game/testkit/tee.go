package testkit

import (
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
)

var ClusterEngines = []sim.StateID{World, Zone1, Zone2, Layer1, Layer2, Layer3}

// NewTeeWorld is NewWorld with a event log that writes the records both in memory and to files.
func NewTeeWorld(
	t *testing.T, dir string,
) (*Cluster, *eventlog.MemLog, *eventlog.FileLog) {
	t.Helper()
	mem := &eventlog.MemLog{}
	file := eventlog.NewFileLog(
		dir, eventlog.FileLogOptions{Seed: 7, SegmentBytes: 2048, SyncEvery: time.Hour, Backlog: 8},
	)
	o := Options(t, 7)
	o.EventLog = teeLog{mem, file}
	return NewCluster(t, o, 10), mem, file
}

// teeLog gives every record to two event logs.
type teeLog struct{ a, b eventlog.Log }

func (t teeLog) Append(e sim.StateID, tick uint64, body eventlog.Body) {
	t.a.Append(e, tick, body)
	t.b.Append(e, tick, body)
}

// Dir is the directory of the second log, which writes files: the orchestrator of the cluster
// stores its file there.
func (t teeLog) Dir() string {
	if directory, ok := t.b.(interface{ Dir() string }); ok {
		return directory.Dir()
	}
	return ""
}

// Safe is the tick that both logs say is safe.
func (t teeLog) Safe(e sim.StateID) uint64 { return min(t.a.Safe(e), t.b.Safe(e)) }

func (t teeLog) EndTick(e sim.StateID, tick uint64) {
	t.a.EndTick(e, tick)
	t.b.EndTick(e, tick)
}
