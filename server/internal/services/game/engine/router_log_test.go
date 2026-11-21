package engine

import (
	"sync"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/shared/logging"
)

type logRecorder struct {
	mu      sync.Mutex
	entries []logging.Entry
}

func (r *logRecorder) Consume(es []logging.Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, es...)
	return nil
}

func TestReturnLogsAWarningWhenTheSenderIsGone(t *testing.T) {
	r := &logRecorder{}
	closeLogs, err := logging.Init(r, logging.WithFlushEvery(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRouter()
	m := action.MessageToZone(1, action.OnFailDrop).Stamp(action.Header{From: action.ZoneAddress(9)})
	rt.Return(m, action.Retired)
	closeLogs()

	if rt.Dropped() != 1 {
		t.Fatalf("dropped %d, want 1", rt.Dropped())
	}
	if len(r.entries) != 1 || r.entries[0].Level != logging.LevelWarn {
		t.Fatalf("entries %+v, want one warning", r.entries)
	}
}
