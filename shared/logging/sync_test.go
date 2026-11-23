package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestTheSyncLoggerWritesEachLineAtOnceWithItsFixedFields(t *testing.T) {
	var buf bytes.Buffer
	l := newSyncLogger(NewWriterConsumer(&buf))
	zone := l.With("zone", 1)

	l.Warn("a")
	if !strings.Contains(buf.String(), "level=warn msg=a\n") {
		t.Fatalf("the line was not written at once: %q", buf.String())
	}
	zone.Error("b", "x", 2)
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[1], "level=error msg=b zone=1 x=2") {
		t.Fatalf("got %q", lines)
	}
}
