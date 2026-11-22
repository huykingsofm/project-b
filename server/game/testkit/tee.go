package testkit

import (
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game"
)

// teeLog gives every record to two replay logs.
type teeLog struct{ a, b replay.Log }

func (t teeLog) Wants(k replay.Kind) bool { return t.a.Wants(k) || t.b.Wants(k) }
func (t teeLog) Describe(e sim.StateID, stateType sim.StateType, name string) {
	t.a.Describe(e, stateType, name)
	t.b.Describe(e, stateType, name)
}
func (t teeLog) Append(e sim.StateID, tick uint64, body replay.Body) {
	t.a.Append(e, tick, body)
	t.b.Append(e, tick, body)
}
func (t teeLog) EndTick(e sim.StateID, tick uint64) {
	t.a.EndTick(e, tick)
	t.b.EndTick(e, tick)
}

var ClusterEngines = []sim.StateID{World, Zone1, Zone2, Layer1, Layer2, Layer3}

// NewTeeWorld is NewWorld with a replay log that writes the records both in memory and to files.
func NewTeeWorld(
	t *testing.T, dir string,
) (*game.Cluster, *replay.MemLog, *replay.FileLog, *config.Store) {
	t.Helper()
	mem := &replay.MemLog{}
	file := replay.NewFileLog(
		dir, replay.FileLogOptions{Seed: 7, SegmentBytes: 2048, SyncEvery: time.Hour, Backlog: 8},
	)
	cfg := config.NewStore()
	c, err := WorldBuilder(t, 7, 10).
		WithConfig(cfg).
		WithReplayLog(teeLog{mem, file}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	return c, mem, file, cfg
}
