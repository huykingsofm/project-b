package engine

import (
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"sync"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/logging"
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
	m := sim.NewActionMessage[*toyState](1, onfail.Drop{}).Stamp(sim.Header{From: 9})
	rt.Return(m, sim.Retired)
	closeLogs()

	if rt.Dropped() != 1 {
		t.Fatalf("dropped %d, want 1", rt.Dropped())
	}
	if len(r.entries) != 1 || r.entries[0].Level != logging.LevelWarn {
		t.Fatalf("entries %+v, want one warning", r.entries)
	}
}
