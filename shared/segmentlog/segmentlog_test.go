package segmentlog_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// opts has a meta of 8 bytes in every segment. The payloads of most of these tests start with a
// tick; the records of a log that starts at the tick 1 have the tick as their ID.
var opts = segmentlog.Options{
	SegmentBytes: 1 << 20, SyncEvery: time.Hour, Backlog: 4, Meta: []byte("testmeta"),
}

// headerSize is the size of the header of a segment of a log made with opts.
var headerSize = 18 + len(opts.Meta) + 4

func open(t testing.TB, dir string, o segmentlog.Options) *segmentlog.Writer {
	t.Helper()
	w, err := segmentlog.Open(dir, o)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// write appends and commits one record for each tick, with a payload that tells its tick.
func write(t testing.TB, w *segmentlog.Writer, from, to uint64) {
	t.Helper()
	for tick := from; tick < to; tick++ {
		if _, err := w.Append(payload(tick)); err != nil {
			t.Fatal(err)
		}
		if err := w.Commit(); err != nil {
			t.Fatal(err)
		}
	}
}

func payload(tick uint64) []byte {
	return fmt.Appendf(binary.LittleEndian.AppendUint64(nil, tick), "record of tick %d", tick)
}

// readAll returns the payloads of the log from a tick, and the error that stopped the read.
func readAll(t testing.TB, dir string, from uint64) ([][]byte, error) {
	t.Helper()
	r, err := segmentlog.OpenReader(dir, from)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var out [][]byte
	for r.Next() {
		out = append(out, bytes.Clone(r.Payload()))
	}
	return out, r.Err()
}

func wantTicks(t *testing.T, got [][]byte, from, to uint64) {
	t.Helper()
	if uint64(len(got)) != to-from {
		t.Fatalf("got %d records, want %d", len(got), to-from)
	}
	for i, p := range got {
		if want := payload(from + uint64(i)); !bytes.Equal(p, want) {
			t.Fatalf("record %d = %q, want %q", i, p, want)
		}
	}
}

func TestRecordsComeBackInOrderWithTheMetaAndTheIndexOfTheirSegment(t *testing.T) {
	dir := t.TempDir()
	w := open(t, dir, opts)
	write(t, w, 10, 60)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := readAll(t, dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantTicks(t, got, 10, 60)

	r, _ := segmentlog.OpenReader(dir, 0)
	defer r.Close()
	if !r.Next() || !bytes.Equal(r.Meta(), opts.Meta) || r.SegmentIndex() != 0 || r.ID() != 1 {
		t.Fatalf("meta %q, segment %d, ID %d", r.Meta(), r.SegmentIndex(), r.ID())
	}
}

func TestSeveralRecordsOfOneCommitAreAllWritten(t *testing.T) {
	dir := t.TempDir()
	w := open(t, dir, opts)
	for i := range uint64(5) {
		w.Append(payload(3 + i))
	}
	w.Commit()
	w.Close()
	got, _ := readAll(t, dir, 0)
	wantTicks(t, got, 3, 8)
}

func TestASegmentIsClosedWhenItIsBigAndTheNextIsNamedByTheNextIndex(t *testing.T) {
	dir := t.TempDir()
	o := opts
	o.SegmentBytes = 200
	w := open(t, dir, o)
	write(t, w, 0, 100)
	w.Close()

	segs, err := segmentlog.Segments(dir)
	if err != nil || len(segs) < 3 {
		t.Fatalf("%d segments, error %v", len(segs), err)
	}
	for i, seg := range segs {
		if seg.Index != uint64(i) {
			t.Fatalf("segments are not numbered 0, 1, 2, ...: %+v", segs)
		}
	}
	if filepath.Base(segs[1].Path) != "seg-00000000000000000001.log" {
		t.Fatalf("name %q", segs[1].Path)
	}
	got, err := readAll(t, dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantTicks(t, got, 0, 100)
}

func TestReadingFromAnIDStartsInTheSegmentThatHoldsItAndGivesTheRecordsFromIt(t *testing.T) {
	dir := t.TempDir()
	o := opts
	o.SegmentBytes = 200
	w := open(t, dir, o)
	write(t, w, 1, 101) // the ID of the record of the tick t is t
	w.Close()

	segs, _ := segmentlog.Segments(dir)
	r, _ := segmentlog.OpenReader(dir, 0)
	starts := map[uint64]uint64{} // the ID each segment begins at
	for r.Next() {
		if _, ok := starts[r.SegmentIndex()]; !ok {
			starts[r.SegmentIndex()] = r.ID()
		}
	}
	r.Close()
	mid := starts[segs[len(segs)/2].Index]
	for _, from := range []uint64{mid, mid + 1, mid - 1, 0, 1, 99, 100, 101, 1000} {
		got, err := readAll(t, dir, from)
		if err != nil {
			t.Fatal(err)
		}
		wantTicks(t, got, min(max(from, 1), 101), 101)
	}
}

func TestACrashThatLeavesHalfARecordIsReadAsTheEndOfTheLogAndRepairedByOpen(t *testing.T) {
	dir := t.TempDir()
	w := open(t, dir, opts)
	write(t, w, 0, 20)
	w.Close()
	segs, _ := segmentlog.Segments(dir)
	path := segs[len(segs)-1].Path
	whole, _ := os.Stat(path)

	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	// a record that says 64 bytes and has 4
	f.Write([]byte{0x40, 0, 0, 0, 1, 2, 3, 4, 'h', 'a', 'l', 'f'})
	f.Close()

	got, err := readAll(t, dir, 0)
	if err != nil {
		t.Fatalf("a torn end is not an error: %v", err)
	}
	wantTicks(t, got, 0, 20)

	w2 := open(t, dir, opts)
	if st, _ := os.Stat(path); st.Size() != whole.Size() {
		t.Fatalf("Open left %d bytes, want %d", st.Size(), whole.Size())
	}
	write(t, w2, 20, 30)
	w2.Close()
	got, err = readAll(t, dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantTicks(t, got, 0, 30)
	if segs, _ := segmentlog.Segments(dir); len(segs) != 2 {
		t.Fatalf("%d segments, want the old one and a new one", len(segs))
	}
}

func TestACorruptRecordInTheMiddleOfTheLogIsAnError(t *testing.T) {
	dir := t.TempDir()
	o := opts
	o.SegmentBytes = 200
	w := open(t, dir, o)
	write(t, w, 0, 100)
	w.Close()
	segs, _ := segmentlog.Segments(dir)
	data, _ := os.ReadFile(segs[0].Path)
	data[len(data)-1] ^= 0xff // the last byte of the first segment
	os.WriteFile(segs[0].Path, data, 0o644)

	got, err := readAll(t, dir, 0)
	if !errors.Is(err, segmentlog.ErrCorrupt) {
		t.Fatalf("error %v, want ErrCorrupt", err)
	}
	if len(got) == 0 || len(got) >= 100 {
		t.Fatalf("%d records read before the damage", len(got))
	}
}

func TestOpenRemovesALastSegmentWithNoRecordsOrANoHeader(t *testing.T) {
	dir := t.TempDir()
	w := open(t, dir, opts)
	write(t, w, 0, 5)
	w.Close()
	os.WriteFile(
		filepath.Join(dir, fmt.Sprintf("seg-%020d.log", 1)), []byte("PBS"), 0o644,
	) // a header that was not finished

	w = open(t, dir, opts)
	w.Close()
	if segs, _ := segmentlog.Segments(dir); len(segs) != 1 {
		t.Fatalf("%d segments left, want 1", len(segs))
	}
	got, err := readAll(t, dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantTicks(t, got, 0, 5)
}

func TestALogThatDoesNotExistIsEmpty(t *testing.T) {
	got, err := readAll(t, filepath.Join(t.TempDir(), "nothing"), 0)
	if err != nil || len(got) != 0 {
		t.Fatalf("got %d records, error %v", len(got), err)
	}
}

func TestAClosedWriterRefusesAndOptionsAndPayloadsThatCannotWorkAreRefused(t *testing.T) {
	w := open(t, t.TempDir(), opts)
	if _, err := w.Append(make([]byte, 1<<24+1)); !errors.Is(err, segmentlog.ErrTooLarge) {
		t.Fatalf("a payload that is too large: %v", err)
	}
	w.Close()
	if err := w.Close(); !errors.Is(err, segmentlog.ErrClosed) {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := w.Append([]byte("x")); !errors.Is(err, segmentlog.ErrClosed) {
		t.Fatalf("Append after Close: %v", err)
	}
	if err := w.Commit(); !errors.Is(err, segmentlog.ErrClosed) {
		t.Fatalf("Commit after Close: %v", err)
	}
	for _, bad := range []segmentlog.Options{{SyncEvery: time.Second}, {SegmentBytes: 1}, {}} {
		if _, err := segmentlog.Open(t.TempDir(), bad); !errors.Is(err, segmentlog.ErrBadOptions) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

func TestAMetaThatIsTooLongIsCut(t *testing.T) {
	dir := t.TempDir()
	o := opts
	o.Meta = make([]byte, 70000)
	w := open(t, dir, o)
	write(t, w, 0, 3)
	w.Close()
	r, _ := segmentlog.OpenReader(dir, 0)
	defer r.Close()
	if !r.Next() || len(r.Meta()) != 1<<16 {
		t.Fatalf("meta of %d bytes", len(r.Meta()))
	}
}

func TestAWriterThatCannotWriteReportsItsFirstErrorAndWritesNothingMore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "log")
	w := open(t, dir, opts)
	os.RemoveAll(dir) // the segment cannot be made
	w.Append([]byte("x"))
	w.Commit()
	deadline := time.Now().Add(5 * time.Second)
	for w.Err() == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if w.Err() == nil {
		t.Fatal("the failure was never reported")
	}
	first := w.Err()
	w.Append([]byte("y"))
	if err := w.Commit(); err != first {
		t.Fatalf("Commit returned %v, want the first error %v", err, first)
	}
	if err := w.Close(); err != first {
		t.Fatalf("Close returned %v, want the first error %v", err, first)
	}
}

func TestACommitThatFindsTheWriterBehindWaitsAndLosesNothing(t *testing.T) {
	dir := t.TempDir()
	o := opts
	o.Backlog = 1
	w := open(t, dir, o)
	const n = 3000
	write(t, w, 0, n)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := readAll(t, dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantTicks(t, got, 0, n)
	t.Logf("commits that had to wait: %d", w.Stalled())
}

func TestManyGoroutinesAppendAndCommitAndTheyAreAllThere(t *testing.T) {
	dir := t.TempDir()
	w := open(t, dir, opts)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 200 {
				w.Append([]byte("x"))
				w.Commit()
			}
		})
	}
	wg.Wait()
	w.Close()
	got, err := readAll(t, dir, 0)
	if err != nil || len(got) != 8*200 {
		t.Fatalf("got %d records, error %v", len(got), err)
	}
}

func TestCloseWhileAnotherGoroutineCommitsNeverPanics(t *testing.T) {
	for range 50 {
		w := open(t, t.TempDir(), opts)
		var wg sync.WaitGroup
		wg.Go(func() {
			for range 100 {
				w.Append([]byte("x"))
				w.Commit()
			}
		})
		w.Close()
		wg.Wait()
	}
}

// BenchmarkAppendOnly is what the caller of Append pays: the time of the Append calls alone, with
// the Commit that hands the batch over, and the goroutine that writes it, left out of the time.
func BenchmarkAppendOnly(b *testing.B) {
	w, err := segmentlog.Open(
		b.TempDir(),
		segmentlog.Options{SegmentBytes: 64 << 20, SyncEvery: time.Second, Backlog: 64},
	)
	if err != nil {
		b.Fatal(err)
	}
	rec := bytes.Repeat([]byte{'x'}, 100)
	b.ReportAllocs()
	b.StopTimer()
	for i := 0; i < b.N; {
		n := min(1000, b.N-i)
		b.StartTimer()
		for range n {
			w.Append(rec)
		}
		b.StopTimer()
		w.Commit()
		i += n
	}
	if err := w.Close(); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkAppendAndCommit is the throughput of the whole path, to a file: records, a Commit for
// each batch, and the goroutine that writes. When the goroutine that writes is the slower one the
// time is its time, not the caller's, and "stalls" counts the Commits that had to wait for it;
// BenchmarkAppendOnly is what the caller pays.
func BenchmarkAppendAndCommit(b *testing.B) {
	for _, perTick := range []int{1, 100, 1000} {
		b.Run(fmt.Sprintf("records per tick=%d", perTick), func(b *testing.B) {
			w, err := segmentlog.Open(
				b.TempDir(),
				segmentlog.Options{SegmentBytes: 64 << 20, SyncEvery: time.Second, Backlog: 64},
			)
			if err != nil {
				b.Fatal(err)
			}
			rec := bytes.Repeat([]byte{'x'}, 100)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i += perTick {
				for range perTick {
					w.Append(rec)
				}
				w.Commit()
			}
			b.StopTimer()
			if err := w.Close(); err != nil {
				b.Fatal(err)
			}
			b.ReportMetric(float64(w.Stalled()), "stalls")
		})
	}
}

func BenchmarkRead(b *testing.B) {
	dir := b.TempDir()
	w, _ := segmentlog.Open(
		dir, segmentlog.Options{SegmentBytes: 64 << 20, SyncEvery: time.Second, Backlog: 64},
	)
	rec := bytes.Repeat([]byte{'x'}, 100)
	for i := range 1 << 16 {
		w.Append(rec)
		if i%100 == 99 {
			w.Commit()
		}
	}
	w.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; {
		r, _ := segmentlog.OpenReader(dir, 0)
		for r.Next() && n < b.N {
			n++
		}
		r.Close()
	}
}

// writeBatches writes a batch for each of the ticks from..to, each with `per` records, all with the
// payload of the tick.
func writeBatches(t testing.TB, w *segmentlog.Writer, from, to uint64, per int) {
	t.Helper()
	for tick := from; tick < to; tick++ {
		for range per {
			if _, err := w.Append(payload(tick)); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Commit(); err != nil {
			t.Fatal(err)
		}
	}
}

// readTicks returns, for each tick of the records that a read gives, how many records it has.
func readTicks(t *testing.T, dir string) (map[uint64]int, error) {
	t.Helper()
	got, err := readAll(t, dir, 0)
	counts := map[uint64]int{}
	for _, p := range got {
		counts[binary.LittleEndian.Uint64(p)]++
	}
	return counts, err
}

// A batch is all or nothing: a write that was cut anywhere, in a record or in the commit that ends
// the batch, leaves the whole batch out of what is read, and Open removes it from the file.
func TestABatchThatWasCutAnywhereIsNotReadAtAll(t *testing.T) {
	const per = 5
	dir := t.TempDir()
	w := open(t, dir, opts)
	writeBatches(t, w, 0, 10, per)
	w.Close()
	segs, _ := segmentlog.Segments(dir)
	path := segs[0].Path
	whole, _ := os.ReadFile(path)
	// the bytes of the last batch: its records and its commit
	oneBatch := len(whole) - len(logBytes(t, 9, per))
	if oneBatch <= 12 {
		t.Fatalf("a batch of %d bytes", oneBatch)
	}
	for cut := 1; cut <= oneBatch; cut++ {
		os.WriteFile(path, whole[:len(whole)-cut], 0o644)
		counts, err := readTicks(t, dir)
		if err != nil {
			t.Fatalf("cut %d: %v", cut, err)
		}
		if len(counts) != 9 || counts[9] != 0 {
			t.Fatalf(
				"cut %d: %d ticks were read, want the 9 whole ones and nothing of the cut one",
				cut,
				len(counts),
			)
		}
		for tick, n := range counts {
			if n != per {
				t.Fatalf("cut %d: tick %d has %d records of %d", cut, tick, n, per)
			}
		}
	}
	// Open removes what was cut, and the log goes on from the whole batches
	os.WriteFile(path, whole[:len(whole)-3], 0o644)
	w = open(t, dir, opts)
	if info, _ := os.Stat(path); int(info.Size()) != len(whole)-oneBatch {
		t.Fatalf("Open left %d bytes, want %d", info.Size(), len(whole)-oneBatch)
	}
	writeBatches(t, w, 20, 22, per)
	w.Close()
	counts, err := readTicks(t, dir)
	if err != nil || len(counts) != 11 || counts[20] != per || counts[21] != per || counts[9] != 0 {
		t.Fatalf("%d ticks after going on, error %v", len(counts), err)
	}
}

// logBytes returns the file of a log of `ticks` batches of `per` records.
func logBytes(t testing.TB, ticks uint64, per int) []byte {
	t.Helper()
	d := t.TempDir()
	w := open(t, d, opts)
	writeBatches(t, w, 0, ticks, per)
	w.Close()
	segs, _ := segmentlog.Segments(d)
	b, _ := os.ReadFile(segs[0].Path)
	return b
}

func TestACommitThatDoesNotMatchItsRecordsEndsTheLogBeforeItsBatch(t *testing.T) {
	dir := t.TempDir()
	w := open(t, dir, opts)
	writeBatches(t, w, 0, 4, 3)
	w.Close()
	segs, _ := segmentlog.Segments(dir)
	data, _ := os.ReadFile(segs[0].Path)
	data[len(data)-1] ^= 0xff // the count of the last commit
	os.WriteFile(segs[0].Path, data, 0o644)
	counts, err := readTicks(t, dir)
	if err != nil || len(counts) != 3 || counts[3] != 0 {
		t.Fatalf(
			"%d ticks, error %v: a commit with a wrong count must leave its batch out",
			len(counts),
			err,
		)
	}
}

func TestACommitWithNoRecordsIsNotWrittenAndABatchOfOneIsWhole(t *testing.T) {
	dir := t.TempDir()
	w := open(t, dir, opts)
	if err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	writeBatches(t, w, 0, 3, 1)
	w.Close()
	counts, err := readTicks(t, dir)
	if err != nil || len(counts) != 3 {
		t.Fatalf("%d ticks, error %v", len(counts), err)
	}
}

// segmentOf returns the path of the last segment of a log.
func segmentOf(t *testing.T, dir string) string {
	t.Helper()
	segs, err := segmentlog.Segments(dir)
	if err != nil || len(segs) == 0 {
		t.Fatalf("no segments: %v", err)
	}
	return segs[len(segs)-1].Path
}

func TestAHeaderThatWasNeverWrittenIsRemovedButOneThatIsNotRightIsKeptAndReported(t *testing.T) {
	for name, content := range map[string][]byte{
		"cut off":            []byte("PBS"),
		"zeros":              make([]byte, 64),
		"another version":    nil, // made below from a real segment
		"crc that is wrong":  nil,
		"another file type":  []byte("this is not a segment of a log, it is a text file"),
		"nothing at all":     {},
		"a header and zeros": nil,
	} {
		dir := t.TempDir()
		w := open(t, dir, opts)
		writeBatches(t, w, 0, 4, 2)
		w.Close()
		whole, _ := os.ReadFile(segmentOf(t, dir))
		next := filepath.Join(dir, "seg-00000000000000000001.log")
		switch name {
		case "another version":
			content = bytes.Clone(whole)
			content[4] = 2
		case "crc that is wrong":
			content = bytes.Clone(whole)
			content[18] ^= 0xff // a byte of the meta
		// the header of a real segment, then zeros where the batch was not written
		case "a header and zeros":
			content = append(bytes.Clone(whole[:headerSize]), make([]byte, 100)...)
		}
		os.WriteFile(next, content, 0o644)

		keep := name == "another "+
			"version" || name == "crc that is wrong" || name == "another file type"
		w2, err := segmentlog.Open(dir, opts)
		if keep {
			if err == nil {
				w2.Close()
				t.Errorf("%s: Open went on with a segment whose header is not right", name)
			}
			if got, _ := os.ReadFile(next); !bytes.Equal(got, content) {
				t.Errorf("%s: the segment was changed or deleted", name)
			}
			if _, err := readAll2(dir); err == nil {
				t.Errorf("%s: a read went on with a last segment whose header is not right", name)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		w2.Close()
		if _, err := os.Stat(next); err == nil && name != "nothing at all" {
			// the new writer made nothing, so the file of an unfinished header must have gone
			t.Errorf("%s: the segment with no header was kept", name)
		}
		counts, err := readTicks(t, dir)
		if err != nil || len(counts) != 4 {
			t.Errorf("%s: %d ticks, error %v, want the 4 whole ones", name, len(counts), err)
		}
	}
}

// readAll2 reads a log from its start and returns its payloads and the error that stopped the read.
func readAll2(dir string) ([][]byte, error) {
	r, err := segmentlog.OpenReader(dir, 0)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var out [][]byte
	for r.Next() {
		out = append(out, bytes.Clone(r.Payload()))
	}
	return out, r.Err()
}

func TestWhatOpenCutsFromTheLastSegmentIsKeptNextToItWhetherItWasATornBatchOrDamageInTheMiddle(
	t *testing.T,
) {
	for name, damage := range map[string]func(whole []byte) []byte{
		"a batch that was cut": func(whole []byte) []byte { return whole[:len(whole)-3] },
		"damage in the middle": func(whole []byte) []byte {
			out := bytes.Clone(whole)
			out[len(out)/2] ^= 0xff
			return out
		},
	} {
		dir := t.TempDir()
		w := open(t, dir, opts)
		writeBatches(t, w, 0, 20, 3)
		w.Close()
		path := segmentOf(t, dir)
		whole, _ := os.ReadFile(path)
		damaged := damage(whole)
		os.WriteFile(path, damaged, 0o644)

		segmentlog.Open(dir, opts) // repairs; its writer is not used
		after, _ := os.ReadFile(path)
		kept, err := filepath.Glob(path + ".discarded-*")
		if err != nil || len(kept) != 1 {
			t.Fatalf("%s: %d files of discarded bytes, want 1", name, len(kept))
		}
		discarded, _ := os.ReadFile(kept[0])
		if len(after)+len(discarded) != len(damaged) ||
			!bytes.Equal(append(bytes.Clone(after), discarded...), damaged) {
			t.Errorf(
				"%s: what was kept and what remains are not what was in the segment: %d + %d "+
					"bytes, was %d",
				name,
				len(after),
				len(discarded),
				len(damaged),
			)
		}
		if len(discarded) == 0 {
			t.Errorf("%s: nothing was kept", name)
		}
	}
}

func TestTheLastSegmentIsReadOnlyUpToItsLastWholeBatchAndTheOthersAreReadWhole(t *testing.T) {
	dir := t.TempDir()
	o := opts
	o.SegmentBytes = 300
	w := open(t, dir, o)
	writeBatches(t, w, 0, 60, 2)
	w.Close()
	segs, _ := segmentlog.Segments(dir)
	if len(segs) < 3 {
		t.Fatalf("%d segments", len(segs))
	}
	last := segs[len(segs)-1].Path
	data, _ := os.ReadFile(last)
	os.WriteFile(last, data[:len(data)-5], 0o644) // cuts the commit of the last batch
	counts, err := readTicks(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	if counts[59] != 0 || len(counts) < 40 {
		t.Fatalf("%d ticks read, tick 59 has %d records", len(counts), counts[59])
	}
	for tick, n := range counts {
		if n != 2 {
			t.Fatalf("tick %d has %d records of 2", tick, n)
		}
	}
}

func TestSyncedCountsTheBatchesThatAreOnTheDisk(t *testing.T) {
	o := opts
	o.SyncEvery = 5 * time.Millisecond
	w := open(t, t.TempDir(), o)
	write(t, w, 0, 5)
	if err := w.Commit(); err != nil { // nothing was appended: not a batch
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for w.Synced() != 5 {
		if time.Now().After(deadline) {
			t.Fatalf("Synced = %d after a while, want 5", w.Synced())
		}
		time.Sleep(time.Millisecond)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if w.Synced() != 5 { // the Commit with nothing appended was not a batch
		t.Fatalf("Synced = %d after Close, want 5", w.Synced())
	}
}

func TestCloseSyncsWhatIsLeftAndSyncedSaysSo(t *testing.T) {
	w := open(t, t.TempDir(), opts) // a sync every hour: only Close syncs
	write(t, w, 0, 3)
	if w.Synced() > 3 {
		t.Fatalf("Synced = %d, more than the 3 batches that were written", w.Synced())
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if w.Synced() != 3 {
		t.Fatalf("Synced = %d after Close, want 3", w.Synced())
	}
}

func TestASegmentThatIsClosedSyncsItsBatches(t *testing.T) {
	o := opts
	o.SegmentBytes = 64 // a segment holds a few batches
	w := open(t, t.TempDir(), o)
	write(t, w, 0, 40)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if w.Synced() != 40 {
		t.Fatalf("Synced = %d, want 40", w.Synced())
	}
}

func TestCutAfterKeepsTheRecordsUpToAnIDAndWhatIsCutIsKept(t *testing.T) {
	for _, last := range []uint64{0, 1, 17, 18, 59, 60, 100} { // across and at the ends of segments
		dir := t.TempDir()
		o := opts
		o.SegmentBytes = 200 // a few batches in a segment
		w := open(t, dir, o)
		write(t, w, 1, 61)
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		if err := segmentlog.CutAfter(dir, last); err != nil {
			t.Fatalf("a cut after %d: %v", last, err)
		}
		got, err := readAll(t, dir, 0)
		if err != nil {
			t.Fatalf("a cut after %d: %v", last, err)
		}
		if want := min(last, 60); uint64(len(got)) != want {
			t.Fatalf("a cut after %d leaves %d records, want %d", last, len(got), want)
		}
		wantTicks(t, got, 1, 1+uint64(len(got)))
		// what was cut is in a file that the log does not see
		if last < 60 {
			if matches, _ := filepath.Glob(filepath.Join(dir, "*.discarded-*")); len(matches) == 0 {
				t.Fatalf("a cut after %d kept nothing of what it cut", last)
			}
		}
		// and the log is written on after it, with the ID after the last one that is left
		w2 := open(t, dir, o)
		if got := w2.LastID(); got != min(last, 60) {
			t.Fatalf("after a cut after %d the last ID is %d", last, got)
		}
		next := min(last, 60) + 1
		write(t, w2, next, next+3)
		if err := w2.Close(); err != nil {
			t.Fatal(err)
		}
		again, err := readAll(t, dir, 0)
		if err != nil || uint64(len(again)) != min(last, 60)+3 {
			t.Fatalf("after a cut after %d and 3 more: %d records, error %v", last, len(again), err)
		}
	}
}

// The cut looks at the headers to find the segment, and reads that one only: a segment before it
// that is damaged is not read, so the cut does not fail on it.
func TestCutAfterReadsOnlyTheSegmentThatHoldsTheCut(t *testing.T) {
	dir := t.TempDir()
	o := opts
	o.SegmentBytes = 200
	w := open(t, dir, o)
	write(t, w, 1, 201)
	w.Close()
	segs, _ := segmentlog.Segments(dir)
	if len(segs) < 6 {
		t.Fatalf("%d segments", len(segs))
	}
	data, _ := os.ReadFile(segs[1].Path)
	data[len(data)-1] ^= 0xff // the commit of the last batch of the second segment
	os.WriteFile(segs[1].Path, data, 0o644)
	if _, err := readAll(t, dir, 0); !errors.Is(err, segmentlog.ErrCorrupt) {
		t.Fatalf("a read of the damaged segment: %v", err)
	}
	if err := segmentlog.CutAfter(dir, 150); err != nil {
		t.Fatalf("a cut after 150, which is in a segment that is not damaged: %v", err)
	}
	got, err := readAll(t, dir, 150)
	if err != nil {
		t.Fatal(err)
	}
	wantTicks(t, got, 150, 151)
}

func TestCutAfterCutsAtTheStartOfTheBatchOfTheFirstRecordThatGoes(t *testing.T) {
	dir := t.TempDir()
	w := open(t, dir, opts)
	writeBatches(t, w, 0, 6, 3) // the tick t has the IDs 3t, 3t+1 and 3t+2
	w.Close()
	if err := segmentlog.CutAfter(dir, 10); err != nil { // 10 is the second record of the tick 3
		t.Fatal(err)
	}
	counts, err := readTicks(t, dir)
	if err != nil || len(counts) != 3 || counts[3] != 0 {
		t.Fatalf("%d ticks, error %v: the batch after the ID 10 must go", len(counts), err)
	}
	w = open(t, dir, opts)
	if last := w.LastID(); last != 9 {
		t.Fatalf("the last ID is %d, want 9", last)
	}
	w.Close()
}

func TestCutAfterThatCutsNothingChangesNothing(t *testing.T) {
	dir := t.TempDir()
	w := open(t, dir, opts)
	write(t, w, 1, 11)
	w.Close()
	before, _ := os.ReadDir(dir)
	if err := segmentlog.CutAfter(dir, 1000); err != nil {
		t.Fatal(err)
	}
	if err := segmentlog.CutAfter(dir, 10); err != nil { // the last record is kept
		t.Fatal(err)
	}
	after, _ := os.ReadDir(dir)
	if len(before) != len(after) {
		t.Fatalf("%d files before and %d after", len(before), len(after))
	}
	if err := segmentlog.CutAfter(filepath.Join(dir, "nothing"), 1); err != nil {
		t.Fatalf("a log that does not exist: %v", err)
	}
}

func TestTheLogNumbersItsRecordsFromOneAndGoesOnFromTheLastOne(t *testing.T) {
	dir := t.TempDir()
	o := opts
	o.SegmentBytes = 100
	w := open(t, dir, o)
	if w.LastID() != 0 || w.Synced() != 0 {
		t.Fatalf("an empty log has the last ID %d and %d synced", w.LastID(), w.Synced())
	}
	for want := uint64(1); want <= 10; want++ {
		if id, err := w.Append(payload(want)); err != nil || id != want {
			t.Fatalf("the record %d got the ID %d, error %v", want, id, err)
		}
		w.Commit()
	}
	if w.LastID() != 10 {
		t.Fatalf("the last ID is %d, want 10", w.LastID())
	}
	w.Close()

	// what the log holds is what the next writer goes on from, and the reader tells the IDs
	if last, err := segmentlog.LastFrameID(dir); err != nil || last != 10 {
		t.Fatalf("LastFrameID = %d, %v, want 10", last, err)
	}
	w = open(t, dir, o)
	if w.LastID() != 10 || w.Synced() != 10 {
		t.Fatalf("a log that was there: last ID %d, %d synced, want 10", w.LastID(), w.Synced())
	}
	if id, _ := w.Append(payload(11)); id != 11 {
		t.Fatalf("the record after the log was opened got the ID %d, want 11", id)
	}
	w.Commit()
	w.Close()
	r, _ := segmentlog.OpenReader(dir, 0)
	defer r.Close()
	n := uint64(0)
	for r.Next() {
		n++
		if r.ID() != n || binary.LittleEndian.Uint64(r.Payload()) != n {
			t.Fatalf("the record %d has the ID %d", n, r.ID())
		}
	}
	if r.Err() != nil || n != 11 {
		t.Fatalf("%d records, error %v", n, r.Err())
	}
}

func TestLastFrameIDOfALogWhoseLastSegmentHasNoBatchIsInThePreviousOne(t *testing.T) {
	dir := t.TempDir()
	if last, err := segmentlog.LastFrameID(filepath.Join(dir, "nothing")); err != nil || last != 0 {
		t.Fatalf("a log that does not exist: %d, %v", last, err)
	}
	o := opts
	o.SegmentBytes = 100
	w := open(t, dir, o)
	write(t, w, 1, 30)
	w.Close()
	segs, _ := segmentlog.Segments(dir)
	os.WriteFile(filepath.Join(dir, fmt.Sprintf("seg-%020d.log", segs[len(segs)-1].Index+1)),
		[]byte("PBS"), 0o644) // a header that was not finished
	if last, err := segmentlog.LastFrameID(dir); err != nil || last != 29 {
		t.Fatalf("LastFrameID = %d, %v, want 29", last, err)
	}
}

func TestOnSyncIsToldHowManyBatchesEachSyncPutOnTheDisk(t *testing.T) {
	var mu sync.Mutex
	var told []uint64
	o := opts // a sync every hour: only Close syncs, so one call holds every batch
	o.OnSync = func(batches uint64) {
		mu.Lock()
		told = append(told, batches)
		mu.Unlock()
	}
	w := open(t, t.TempDir(), o)
	write(t, w, 0, 7)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(told) != 1 || told[0] != 7 {
		t.Fatalf("OnSync was told %v, want one sync of 7 batches", told)
	}
}

func TestTheBatchesThatOnSyncIsToldAddUpToTheOnesOnTheDisk(t *testing.T) {
	var mu sync.Mutex
	var sum uint64
	o := opts
	o.SyncEvery = time.Millisecond
	o.SegmentBytes = 200 // a segment that is closed syncs too
	o.OnSync = func(batches uint64) {
		mu.Lock()
		sum += batches
		mu.Unlock()
	}
	w := open(t, t.TempDir(), o)
	for tick := range uint64(60) {
		write(t, w, tick, tick+1)
		if tick%7 == 0 {
			time.Sleep(2 * time.Millisecond) // let a sync come between some of them
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if sum != 60 || w.Synced() != 60 {
		t.Fatalf("OnSync was told %d batches and Synced is %d, want 60 and 60", sum, w.Synced())
	}
}
