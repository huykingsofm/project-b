package replay

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/checkpoint"
	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// theLine is the recovery line of the files under dir, with the cache of the line when it holds.
func theLine(dir string) (map[sim.StateID]uint64, error) {
	logs, err := eventlog.NewFileLogs(dir, toyCodecs())
	if err != nil {
		return nil, err
	}
	last, err := lastTicks(logs)
	if err != nil {
		return nil, err
	}
	return recoveryLine(logs, last, knownSafe(dir, last))
}

// cutToTheLine cuts the files under dir to the recovery line and returns it.
func cutToTheLine(dir string) (map[sim.StateID]uint64, error) {
	p, err := MakePlan(dir, toyCodecs())
	if err != nil {
		return nil, err
	}
	return p.Limits(), p.Cut()
}

// startsOf is where each engine of limits starts, from the checkpoints under dir.
func startsOf(dir string, limits map[sim.StateID]uint64) (map[sim.StateID]uint64, error) {
	logs, err := eventlog.NewFileLogs(dir, toyCodecs())
	if err != nil {
		return nil, err
	}
	return chooseStarts(dir, logs, limits)
}

var testFileLogOptions = eventlog.FileLogOptions{
	Seed: 42, SegmentBytes: 1 << 20, SyncEvery: time.Hour, Backlog: 4,
}

// ticks writes the ticks 1 to to of an engine, each with the record that at says (none for a tick
// that is not in it).
func ticks(log *eventlog.FileLog, engine sim.StateID, to uint64, at map[uint64]eventlog.Body) {
	for tick := uint64(1); tick <= to; tick++ {
		if body, ok := at[tick]; ok {
			log.Append(engine, tick, body)
		}
		log.EndTick(engine, tick)
	}
}

func lastTicksOf(t *testing.T, dir string, engines ...sim.StateID) map[sim.StateID]uint64 {
	t.Helper()
	out := map[sim.StateID]uint64{}
	for _, e := range engines {
		last, err := segmentlog.LastFrameID(eventlog.EngineDir(dir, e))
		if err != nil {
			t.Fatal(err)
		}
		out[e] = last
	}
	return out
}

// checkpoints writes a file for each tick of an engine, which holds nothing a test reads.
func checkpoints(t *testing.T, dir string, engine sim.StateID, at ...uint64) {
	t.Helper()
	w := checkpoint.NewFileWriter(dir, len(at)+1)
	for _, tick := range at {
		w.Offer(engine, tick, func() ([]byte, error) { return []byte("checkpoint"), nil })
	}
	w.Close()
	if w.Written() != uint64(len(at)) {
		t.Fatalf("%d checkpoints written, want %d", w.Written(), len(at))
	}
}

func TestTheRecoveryLineCutsAnEngineBeforeATickThatDependsOnATickThatWasLost(t *testing.T) {
	dir := t.TempDir()
	a, b, c := sim.StateID(1), sim.StateID(2), sim.StateID(3)
	d, lone := sim.StateID(4), sim.StateID(8)
	log := eventlog.NewFileLog(dir, testFileLogOptions)
	// a was lost after its tick 5; b took at its tick 4 a message that a sent at 6; c took at its
	// tick 5 a message that b sent at 5, which b lost; d read at 2 a copy of an engine that has no
	// log; lone depends on nothing
	ticks(log, a, 5, nil)
	ticks(log, b, 6, map[uint64]eventlog.Body{4: eventlog.Inbound{Message: aMessageFrom(a, 1, 6)}})
	ticks(log, c, 7, map[uint64]eventlog.Body{
		5: eventlog.Inbound{Message: aMessageFrom(b, 1, 5)},
		2: eventlog.Inbound{Message: aMessageFrom(a, 2, 5)}, // a sent it at 5, which is kept
	})
	ticks(log, d, 4, map[uint64]eventlog.Body{2: eventlog.StableRead{Source: 99, CopyTick: 1}})
	ticks(log, lone, 9, nil)
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := theLine(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[sim.StateID]uint64{a: 5, b: 3, c: 4, d: 1, lone: 9}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the recovery line is %v, want %v", got, want)
	}
}

func TestACutToTheRecoveryLineLeavesLogsThatEndAtThePointAndKeepsWhatItCutInTheArchive(
	t *testing.T,
) {
	dir := t.TempDir()
	a, b := sim.StateID(1), sim.StateID(2)
	log := eventlog.NewFileLog(dir, testFileLogOptions)
	ticks(log, a, 3, nil)
	ticks(log, b, 8, map[uint64]eventlog.Body{
		5: eventlog.Inbound{Message: aMessageFrom(a, 1, 4)}, // a was lost after 3
	})
	log.Close()
	// b has checkpoints on both sides of its line, and a has one at its line
	checkpoints(t, dir, b, 2, 4, 6)
	checkpoints(t, dir, a, 3)
	limits, err := cutToTheLine(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[sim.StateID]uint64{a: 3, b: 4}
	if !reflect.DeepEqual(limits, want) || !reflect.DeepEqual(lastTicksOf(t, dir, a, b), want) {
		t.Fatalf("limits %v, last ticks %v, want %v", limits, lastTicksOf(t, dir, a, b), want)
	}
	archive := filepath.Join(dir, ".archive", "cut-1", filepath.Base(eventlog.EngineDir(dir, b)))
	if matches, _ := filepath.Glob(filepath.Join(archive, "*.discarded-*")); len(matches) == 0 {
		t.Fatalf("what was cut of the log of b is not in %s", archive)
	}
	next, _ := filepath.Glob(filepath.Join(eventlog.EngineDir(dir, b), "*.discarded-*"))
	if len(next) != 0 {
		t.Fatalf("what was cut is left next to the segments: %v", next)
	}
	if got, _ := checkpoint.Ticks(dir, b); !reflect.DeepEqual(got, []uint64{2, 4}) {
		t.Fatalf("b has the checkpoints %v, want 2 and 4: the one at 6 is above its line", got)
	}
	if got, _ := checkpoint.Ticks(dir, a); !reflect.DeepEqual(got, []uint64{3}) {
		t.Fatalf("a has the checkpoints %v, want 3", got)
	}
	kept, _ := filepath.Glob(filepath.Join(archive, "ckpt-00000000000000000006.*.snap"))
	if len(kept) != 1 {
		t.Fatalf("the checkpoint above the line is not in the archive: %v", kept)
	}
	if engines, _ := eventlog.Engines(dir); len(engines) != 2 {
		t.Fatalf("the archive is read as a log: engines %v", engines)
	}
	if cache, found, err := ReadLineCache(dir); err != nil || !found ||
		!reflect.DeepEqual(cache.Safe, want) {
		t.Fatalf("the cache is %v (%v, %v), want %v", cache.Safe, found, err, want)
	}
	again, err := cutToTheLine(dir)
	if err != nil || !reflect.DeepEqual(again, want) {
		t.Fatalf("a second cut gave %v, %v", again, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".archive", "cut-2")); err == nil {
		t.Fatal("a cut that had nothing to cut made an archive directory")
	}
	// the log that was cut is written on after its last tick
	reopened := eventlog.NewFileLog(dir, testFileLogOptions)
	defer reopened.Close()
	reopened.EndTick(b, 5)
	if reopened.Err() != nil {
		t.Fatalf("the log that was cut could not be written on: %v", reopened.Err())
	}
}

func TestTwoCutsKeepWhatTheyCutInDirectoriesOfTheirOwn(t *testing.T) {
	dir := t.TempDir()
	a, b := sim.StateID(1), sim.StateID(2)
	// a was lost after its tick 3, and b took at its tick 6 a message that a sent at 5
	log := eventlog.NewFileLog(dir, testFileLogOptions)
	ticks(log, a, 3, nil)
	ticks(log, b, 9, map[uint64]eventlog.Body{6: eventlog.Inbound{Message: aMessageFrom(a, 1, 5)}})
	log.Close()
	checkpoints(t, dir, b, 8)
	if _, err := cutToTheLine(dir); err != nil {
		t.Fatal(err)
	}
	// b goes on from its tick 5 and takes the same message again, as it does when a is lost again
	// in the same way, and takes a checkpoint at the same tick
	log = eventlog.NewFileLog(dir, testFileLogOptions)
	for tick := uint64(6); tick <= 9; tick++ {
		if tick == 6 {
			log.Append(b, tick, eventlog.Inbound{Message: aMessageFrom(a, 1, 5)})
		}
		log.EndTick(b, tick)
	}
	log.Close()
	checkpoints(t, dir, b, 8)
	limits, err := cutToTheLine(dir)
	if err != nil {
		t.Fatal(err)
	}
	if limits[b] != 5 {
		t.Fatalf("b is cut at %d, want 5", limits[b])
	}
	for _, n := range []string{"cut-1", "cut-2"} {
		engineDir := filepath.Base(eventlog.EngineDir(dir, b))
		engineArchive := filepath.Join(dir, ".archive", n, engineDir)
		kept, _ := filepath.Glob(filepath.Join(engineArchive, "ckpt-00000000000000000008.*.snap"))
		if len(kept) != 1 {
			t.Fatalf("the checkpoint of %s is not kept: %v", n, kept)
		}
	}
}

func TestTheCacheOfTheLineSavesReadingTheLogUpToWhatItSaysIsSafe(t *testing.T) {
	dir := t.TempDir()
	a, b := sim.StateID(1), sim.StateID(2)
	opts := testFileLogOptions
	opts.SegmentBytes = 200 // a few ticks in a segment
	log := eventlog.NewFileLog(dir, opts)
	ticks(log, a, 60, nil)
	ticks(log, b, 60, map[uint64]eventlog.Body{
		55: eventlog.Inbound{Message: aMessageFrom(a, 1, 50)},
	})
	log.Close()
	want := map[sim.StateID]uint64{a: 60, b: 60}
	if got, err := theLine(dir); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("without a cache: %v, %v, want %v", got, err, want)
	}
	// the cache says 40 of each is safe; the first segment of each is damaged, which only a read
	// from the start would find
	forty := LineCache{Safe: map[sim.StateID]uint64{a: 40, b: 40}}
	if err := WriteLineCache(dir, forty); err != nil {
		t.Fatal(err)
	}
	for _, e := range []sim.StateID{a, b} {
		segments, err := segmentlog.Segments(eventlog.EngineDir(dir, e))
		if err != nil || len(segments) < 4 {
			t.Fatalf("%d segments (%v)", len(segments), err)
		}
		data, _ := os.ReadFile(segments[0].Path)
		data[len(data)-1] ^= 0xff
		if err := os.WriteFile(segments[0].Path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := theLine(dir); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("with the cache: %v, %v, want %v", got, err, want)
	}
	// a cache that says more than a log holds is not used: the logs are read whole, and the damage
	// is found
	tooMuch := LineCache{Safe: map[sim.StateID]uint64{a: 61, b: 40}}
	if err := WriteLineCache(dir, tooMuch); err != nil {
		t.Fatal(err)
	}
	if _, err := theLine(dir); err == nil {
		t.Fatal("a cache that says more than the log holds was used")
	}
	// so is one that names an engine that has no log, and one that is cut
	for _, bad := range []LineCache{
		{Safe: map[sim.StateID]uint64{a: 40, b: 40, 9: 1}},
	} {
		if err := WriteLineCache(dir, bad); err != nil {
			t.Fatal(err)
		}
		if _, err := theLine(dir); err == nil {
			t.Fatalf("the cache %v was used", bad.Safe)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, lineCacheFile), []byte{1, 5}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := theLine(dir); err == nil {
		t.Fatal("a cache that is cut was used, and the damaged logs were read")
	}
}

func TestACacheThatIsBadIsAnErrorWhenReadAndNeverAPanic(t *testing.T) {
	dir := t.TempDir()
	if _, found, err := ReadLineCache(dir); found || err != nil {
		t.Fatalf("no file: found %v, err %v", found, err)
	}
	whole := LineCache{Safe: map[sim.StateID]uint64{1: 5, 2: 9}}.encode()
	for name, data := range map[string][]byte{
		"cut":         whole[:len(whole)-1],
		"a byte too":  append(append([]byte{}, whole...), 0),
		"version":     append([]byte{9}, whole[1:]...),
		"too many":    {1, 200, 1},
		"an ID twice": {1, 2, 1, 5, 1, 6},
	} {
		if err := os.WriteFile(filepath.Join(dir, lineCacheFile), data, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, found, err := ReadLineCache(dir); found || err == nil {
			t.Fatalf("%s: found %v, err %v", name, found, err)
		}
	}
	if err := WriteLineCache(dir, LineCache{Safe: map[sim.StateID]uint64{1: 5, 2: 9}}); err != nil {
		t.Fatal(err)
	}
	got, found, err := ReadLineCache(dir)
	if err != nil || !found || !reflect.DeepEqual(got.Safe, map[sim.StateID]uint64{1: 5, 2: 9}) {
		t.Fatalf("read back %v (%v, %v)", got.Safe, found, err)
	}
}

// fixedConfig is a ConfigSource that always gives the same snapshot.
type fixedConfig struct{ snapshot config.ReadOnlySnapshot }

func (f fixedConfig) Config() config.ReadOnlySnapshot { return f.snapshot }

func TestTheWriterOfTheCacheWritesWhenAnEngineIsAPeriodAheadAndAtTheEnd(t *testing.T) {
	dir := t.TempDir()
	snapshot, _, err := config.Build(map[string]string{"replay.line_cache_every_ticks": "10"})
	if err != nil {
		t.Fatal(err)
	}
	w := NewLineCacheWriter(dir, fixedConfig{snapshot})
	read := func() map[sim.StateID]uint64 {
		c, found, err := ReadLineCache(dir)
		if err != nil || !found {
			return nil
		}
		return c.Safe
	}
	w.SafeChanged(map[sim.StateID]uint64{1: 4, 2: 9}, false)
	if got := read(); got != nil {
		t.Fatalf("written before a period: %v", got)
	}
	w.SafeChanged(map[sim.StateID]uint64{1: 4, 2: 10}, false)
	if got := read(); !reflect.DeepEqual(got, map[sim.StateID]uint64{1: 4, 2: 10}) {
		t.Fatalf("after a period: %v", got)
	}
	w.SafeChanged(map[sim.StateID]uint64{1: 12, 2: 19}, false) // 8 and 9 past what was written
	if got := read(); !reflect.DeepEqual(got, map[sim.StateID]uint64{1: 4, 2: 10}) {
		t.Fatalf("written before a period: %v", got)
	}
	w.SafeChanged(map[sim.StateID]uint64{1: 12, 2: 19}, true)
	if got := read(); !reflect.DeepEqual(got, map[sim.StateID]uint64{1: 12, 2: 19}) {
		t.Fatalf("at the end: %v", got)
	}
}

func TestALogThatTellsTheWriterOfTheCacheLeavesTheCacheOfWhatWasSafeWhenItClosed(t *testing.T) {
	dir := t.TempDir()
	a, b := sim.StateID(1), sim.StateID(2)
	opts := testFileLogOptions
	opts.OnSafe = NewLineCacheWriter(dir, nil)
	log := eventlog.NewFileLog(dir, opts)
	ticks(log, a, 5, nil)
	ticks(log, b, 8, nil)
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	cache, found, err := ReadLineCache(dir)
	if err != nil || !found || !reflect.DeepEqual(cache.Safe, map[sim.StateID]uint64{a: 5, b: 8}) {
		t.Fatalf("the cache is %v (%v, %v)", cache.Safe, found, err)
	}
}

func TestAStartIsTheLatestCheckpointAtOrBelowTheLimitAndGoesBackWhereACopyIsReadBeforeIt(
	t *testing.T,
) {
	dir := t.TempDir()
	a, b, c := sim.StateID(1), sim.StateID(2), sim.StateID(3)
	log := eventlog.NewFileLog(dir, testFileLogOptions)
	// b reads at its tick 25 a copy of a at 12, and at its tick 15, before its checkpoint at 20,
	// one of c at 3; c reads at 28 a copy of b at 18
	ticks(log, a, 30, nil)
	ticks(log, b, 30, map[uint64]eventlog.Body{
		25: eventlog.StableRead{Source: a, CopyTick: 12},
		15: eventlog.StableRead{Source: c, CopyTick: 3},
	})
	ticks(log, c, 30, map[uint64]eventlog.Body{28: eventlog.StableRead{Source: b, CopyTick: 18}})
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	checkpoints(t, dir, a, 10, 20, 30)
	checkpoints(t, dir, b, 20)
	checkpoints(t, dir, c, 6, 24)
	limits := map[sim.StateID]uint64{a: 30, b: 30, c: 28}
	starts, err := startsOf(dir, limits)
	if err != nil {
		t.Fatal(err)
	}
	// c starts at 24 and reads b at 18 at its tick 28: b starts before 18, where it has none, so at
	// 0; b then reads c at 3 at its tick 15 and a at 12 at its tick 25: a starts at 10, and c at 0,
	// since its checkpoint at 6 is above 3
	want := map[sim.StateID]uint64{a: 10, b: 0, c: 0}
	if !reflect.DeepEqual(starts, want) {
		t.Fatalf("the starts are %v, want %v", starts, want)
	}
}

func TestAStartThatNoCopyIsReadBeforeIsTheLatestCheckpoint(t *testing.T) {
	dir := t.TempDir()
	a, b := sim.StateID(1), sim.StateID(2)
	log := eventlog.NewFileLog(dir, testFileLogOptions)
	// b reads at its tick 15, before its checkpoint at 20, a copy of a at 12: it is in the
	// checkpoint already
	ticks(log, a, 30, nil)
	ticks(log, b, 30, map[uint64]eventlog.Body{15: eventlog.StableRead{Source: a, CopyTick: 12}})
	log.Close()
	checkpoints(t, dir, a, 10, 20)
	checkpoints(t, dir, b, 20)
	starts, err := startsOf(dir, map[sim.StateID]uint64{a: 30, b: 30})
	if err != nil {
		t.Fatal(err)
	}
	if want := map[sim.StateID]uint64{a: 20, b: 20}; !reflect.DeepEqual(starts, want) {
		t.Fatalf("the starts are %v, want %v", starts, want)
	}
}
