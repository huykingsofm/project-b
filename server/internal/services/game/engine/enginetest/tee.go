package enginetest

import (
	"testing"
	"time"

	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
)

// teeLog gives every record to two replay logs.
type teeLog struct{ a, b replay.Log }

func (t teeLog) Wants(k replay.Kind) bool { return t.a.Wants(k) || t.b.Wants(k) }
func (t teeLog) Append(e message.Address, tick uint64, body replay.Body) {
	t.a.Append(e, tick, body)
	t.b.Append(e, tick, body)
}
func (t teeLog) EndTick(e message.Address, tick uint64) {
	t.a.EndTick(e, tick)
	t.b.EndTick(e, tick)
}

var ClusterEngines = []message.Address{
	message.WorldAddress(1),
	message.ZoneAddress(1), message.ZoneAddress(2),
	message.ZoneLayerAddress(1), message.ZoneLayerAddress(2), message.ZoneLayerAddress(3),
}

// NewTeeWorld is NewWorld with a replay log that writes the records both in memory and to files.
func NewTeeWorld(
	t *testing.T, dir string,
) (*engine.Cluster, *replay.MemLog, *replay.FileLog, *config.Store) {
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
