package testkit

import (
	"sync"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/shared/logging"
)

// RecordLogs makes the log of the process keep its entries, and returns the function that stops
// that, and gives them. A test calls it once, where it reads the entries; the entries of the
// other tests are not in it.
func RecordLogs(t testing.TB) (finish func() []logging.Entry) {
	t.Helper()
	recorder := &logRecorder{}
	closeLogs, err := logging.Init(recorder, logging.WithFlushEvery(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return func() []logging.Entry {
		closeLogs()
		return recorder.entries
	}
}

// LogField is the value of the field of an entry with the key, and false when it has none.
func LogField(entry logging.Entry, key string) (any, bool) {
	for i := 0; i+1 < len(entry.Fields); i += 2 {
		if entry.Fields[i] == key {
			return entry.Fields[i+1], true
		}
	}
	return nil, false
}

// logRecorder is a consumer of the log that keeps what it is given.
type logRecorder struct {
	mu      sync.Mutex
	entries []logging.Entry
}

func (r *logRecorder) Consume(entries []logging.Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, entries...)
	return nil
}
