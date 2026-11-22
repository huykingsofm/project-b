package logging_test

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/shared/logging"
)

// recorder is a Consumer that keeps every entry it is given.
type recorder struct {
	mu      sync.Mutex
	entries []logging.Entry
	batches int
	err     error
}

func (r *recorder) Consume(es []logging.Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, es...)
	r.batches++
	return r.err
}

func (r *recorder) got() []logging.Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]logging.Entry(nil), r.entries...)
}

// start runs Init with a flush period so long that only the closing function flushes.
func start(t *testing.T, c logging.Consumer, opts ...logging.Option) func() {
	t.Helper()
	opts = append([]logging.Option{logging.WithFlushEvery(time.Hour)}, opts...)
	closeLogs, err := logging.Init(c, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeLogs)
	return closeLogs
}

func messages(es []logging.Entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Level.String() + ":" + e.Message
	}
	return out
}

func TestPackageFunctionsWriteThroughTheGlobalLoggerInOrder(t *testing.T) {
	r := &recorder{}
	closeLogs := start(t, r)
	logging.Debug("d")
	logging.Info("i", "zone", 1)
	logging.Warn("w")
	logging.Error("e", "err", errors.New("boom"))
	closeLogs()
	want := "debug:d info:i warn:w error:e"
	if got := strings.Join(messages(r.got()), " "); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if f := r.got()[1].Fields; len(f) != 2 || f[0] != "zone" || f[1] != 1 {
		t.Fatalf("fields %v", f)
	}
}

func TestWithMakesALoggerWhoseLinesStartWithItsFieldsAndLeavesTheParentAlone(t *testing.T) {
	r := &recorder{}
	closeLogs := start(t, r)
	zone := logging.With("zone", 1)
	layer := zone.With("layer", 2)
	logging.Info("a")
	zone.Warn("b", "x", 1)
	layer.Error("c")
	closeLogs()
	es := r.got()
	for i, want := range [][]any{nil, {"zone", 1, "x", 1}, {"zone", 1, "layer", 2}} {
		if len(es[i].Fields) != len(want) {
			t.Fatalf("entry %d fields %v, want %v", i, es[i].Fields, want)
		}
		for j := range want {
			if es[i].Fields[j] != want[j] {
				t.Fatalf("entry %d fields %v, want %v", i, es[i].Fields, want)
			}
		}
	}
}

func TestBeforeInitAndAfterCloseTheGlobalLoggerStillWorks(t *testing.T) {
	logging.Info("before init") // goes to stderr at once; must not panic or block
	r := &recorder{}
	closeLogs := start(t, r)
	logging.Info("kept")
	closeLogs()
	logging.Info("after close") // goes to stderr, not to r
	if got := messages(r.got()); len(got) != 1 || got[0] != "info:kept" {
		t.Fatalf("got %q", got)
	}
}

func TestInitRefusesANilConsumerAndASecondInitUntilTheFirstIsClosed(t *testing.T) {
	if _, err := logging.Init(nil); !errors.Is(err, logging.ErrNoConsumer) {
		t.Fatalf("nil consumer: %v", err)
	}
	closeLogs := start(t, &recorder{})
	if _, err := logging.Init(&recorder{}); !errors.Is(err, logging.ErrAlreadyInitialized) {
		t.Fatalf("second init: %v", err)
	}
	closeLogs()
	closeLogs() // closing twice is fine
	again, err := logging.Init(&recorder{})
	if err != nil {
		t.Fatalf("init after close: %v", err)
	}
	again()
}

func TestTheConsumerIsCalledOnItsOwnEveryFlushPeriod(t *testing.T) {
	r := &recorder{}
	closeLogs, err := logging.Init(r, logging.WithFlushEvery(5*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	defer closeLogs()
	logging.Info("hello")
	deadline := time.Now().Add(2 * time.Second)
	for len(r.got()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the consumer was never called")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestAFullQueueDropsAndTheNextBatchSaysHowMany(t *testing.T) {
	r := &recorder{}
	closeLogs := start(t, r, logging.WithCapacity(2))
	for i := range 5 {
		logging.Info("m", "i", i)
	}
	closeLogs()
	es := r.got()
	if got := strings.Join(messages(es), " "); got != "warn:log entries dropped info:m info:m" {
		t.Fatalf("got %q", got)
	}
	if es[0].Fields[1] != uint64(3) {
		t.Fatalf("dropped %v, want 3", es[0].Fields[1])
	}
}

func TestAFailingConsumerDoesNotStopTheLog(t *testing.T) {
	r := &recorder{err: errors.New("disk full")}
	closeLogs := start(t, r)
	logging.Info("a")
	closeLogs() // the error goes to stderr; no panic
	if len(r.got()) != 1 {
		t.Fatal("the consumer was not given the entry")
	}
}

func TestManyGoroutinesLogWhileTheLogFlushes(t *testing.T) {
	r := &recorder{}
	closeLogs, err := logging.Init(
		r, logging.WithFlushEvery(time.Millisecond), logging.WithCapacity(1<<16),
	)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			l := logging.With("g", g)
			for i := range 500 {
				l.Info("m", "i", i)
				logging.Warn("w")
			}
		})
	}
	wg.Wait()
	closeLogs()
	if n := len(r.got()); n != 8*500*2 {
		t.Fatalf("got %d entries, want %d", n, 8*500*2)
	}
}

var timeField = regexp.MustCompile(`^time=\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{6}Z `)

// encode writes entries through the writer consumer and returns the lines without the time field.
func encode(t *testing.T, es ...logging.Entry) []string {
	t.Helper()
	var buf bytes.Buffer
	if err := logging.NewWriterConsumer(&buf).Consume(es); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	for i, l := range lines {
		if !timeField.MatchString(l) {
			t.Fatalf("line %q does not start with a time", l)
		}
		lines[i] = timeField.ReplaceAllString(l, "")
	}
	return lines
}

func entry(level logging.Level, msg string, kv ...any) logging.Entry {
	return logging.Entry{Time: time.Now(), Level: level, Message: msg, Fields: kv}
}

func TestEveryLineHasTimeLevelAndMessageThenTheFieldsInOrder(t *testing.T) {
	got := encode(t,
		entry(logging.LevelDebug, "d"),
		entry(logging.LevelInfo, "i", "zone", 1, "name", "a"),
		entry(logging.LevelWarn, "w", "ok", true, "ratio", 0.5),
		entry(logging.LevelError, "e", "err", errors.New("boom"), "after", 2*time.Second),
	)
	want := []string{
		"level=debug msg=d",
		"level=info msg=i zone=1 name=a",
		"level=warn msg=w ok=true ratio=0.5",
		"level=error msg=e err=boom after=2s",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestTheTimeIsUTCWithMicroseconds(t *testing.T) {
	var buf bytes.Buffer
	at := time.Date(2026, 11, 21, 10, 0, 0, 123456789, time.FixedZone("x", 7*3600))
	_ = logging.NewWriterConsumer(&buf).Consume(
		[]logging.Entry{{Time: at, Level: logging.LevelInfo, Message: "m"}},
	)
	if want := "time=2026-11-21T03:00:00.123456Z level=info msg=m\n"; buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

func TestValuesThatCouldBreakALineAreQuoted(t *testing.T) {
	got := encode(
		t,
		entry(
			logging.LevelInfo,
			"two words",
			"empty",
			"",
			"eq",
			"a=b",
			"quote",
			`say "hi"`,
			"nl",
			"a\nb",
			"nothing",
			nil,
		),
	)
	want := `level=info msg="two words" empty="" eq="a=b" quote="say \"hi\"" nl="a\nb" nothing=null`
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestABadKeyOrAMissingValueNeverBreaksTheLine(t *testing.T) {
	got := encode(t, entry(logging.LevelInfo, "m", 5, "x", "a b", 1, "", 2, "last"))
	want := `level=info msg=m !BADKEY:5=x a_b=1 !EMPTYKEY=2 last=!MISSING`
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestTheWriterConsumerReturnsTheErrorOfTheWriter(t *testing.T) {
	if err := logging.NewWriterConsumer(failingWriter{}).Consume(
		[]logging.Entry{entry(logging.LevelInfo, "a")},
	); err == nil {
		t.Fatal("the error of the writer was lost")
	}
}

type discardConsumer struct{}

func (discardConsumer) Consume([]logging.Entry) error { return nil }

func BenchmarkGlobalCall(b *testing.B) {
	closeLogs, _ := logging.Init(
		discardConsumer{}, logging.WithCapacity(b.N+1), logging.WithFlushEvery(time.Hour),
	)
	defer closeLogs()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logging.Info("character moved", "zone", 1, "id", i)
	}
}

func BenchmarkWithLoggerCall(b *testing.B) {
	closeLogs, _ := logging.Init(
		discardConsumer{}, logging.WithCapacity(b.N+1), logging.WithFlushEvery(time.Hour),
	)
	defer closeLogs()
	l := logging.With("zone", 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Info("character moved", "id", i)
	}
}

func BenchmarkEncodePerEntry(b *testing.B) {
	c := logging.NewWriterConsumer(io.Discard)
	batch := make([]logging.Entry, 1024)
	for i := range batch {
		batch[i] = entry(logging.LevelInfo, "character moved", "zone", 1, "id", i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(batch) {
		_ = c.Consume(batch)
	}
}
