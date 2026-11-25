package eventlog

import (
	"fmt"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// engineDir is the directory of the log of an engine that did not describe itself to the log.
func engineDir(dir string, engine sim.StateID) string {
	return filepath.Join(dir, engineDirName(engine))
}

var testFileLogOptions = FileLogOptions{
	Seed: 42, SegmentBytes: 1 << 20, SyncEvery: time.Hour, Backlog: 4,
}

func readLog(t *testing.T, dir string, engine sim.StateID, from uint64) ([]Record, error) {
	t.Helper()
	r, err := ReadFileLog(dir, engine, from, toyCodecs())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var out []Record
	for r.Next() {
		out = append(out, r.Record())
	}
	return out, r.Err()
}

// recordsOfEveryKind writes a record of every kind for an engine at a tick, and returns what it
// wrote.
func recordsOfEveryKind(log Log, engine sim.StateID, tick uint64) []Record {
	from := sim.StateID(3)
	inbound := sim.NewActionMessage[*toyState](
		3, onfail.Compensate{With: sim.NewActionMessage[*toyState](
			1, onfail.Drop{}, toyRegenAction{},
		)},
		toyMoveAction{CharacterID: 7, Dir: 1, Ticks: 3},
		toyChatAction{FromID: 7, Text: "xin chào"},
	).Stamp(sim.Header{From: from, Seq: tick * 10})
	enter := toyEnter{Name: "Binh", Cooldown: tick + 6}
	move := toyMove{CharacterID: 7, Dir: 1, Ticks: 3}
	chat := toyChat{FromID: 1, Text: "psst"}

	log.Append(engine, tick, ConfigRead{Version: 5})
	log.Append(engine, tick, Inbound{Message: inbound})
	log.Append(engine, tick, Returned{
		Header: sim.Header{From: from, Seq: tick*10 + 1}, Reason: sim.DeliveryFailureFull,
	})
	log.Append(engine, tick, Command{Command: chat})
	log.Append(engine, tick, Command{Command: enter})
	log.Append(engine, tick, Command{Command: move})
	log.Append(engine, tick, StableRead{Source: sim.StateID(12), CopyTick: tick - 1})
	log.Append(engine, tick, Hash{Value: 0xdeadbeefcafef00d + tick})
	log.EndTick(engine, tick)
	return []Record{
		{Engine: engine, Tick: tick, Body: ConfigRead{Version: 5}},
		{Engine: engine, Tick: tick, Body: Inbound{Message: inbound}},
		{
			Engine: engine,
			Tick:   tick,
			Body: Returned{
				Header: sim.Header{From: from, Seq: tick*10 + 1}, Reason: sim.DeliveryFailureFull,
			},
		},
		{Engine: engine, Tick: tick, Body: Command{Command: chat}},
		{Engine: engine, Tick: tick, Body: Command{Command: enter}},
		{Engine: engine, Tick: tick, Body: Command{Command: move}},
		{
			Engine: engine,
			Tick:   tick,
			Body:   StableRead{Source: sim.StateID(12), CopyTick: tick - 1},
		},
		{Engine: engine, Tick: tick, Body: Hash{Value: 0xdeadbeefcafef00d + tick}},
	}
}

func TestEveryKindOfRecordComesBackAsItWasWritten(t *testing.T) {
	dir := t.TempDir()
	log := NewFileLog(dir, testFileLogOptions)
	engine := sim.StateID(1)
	var want []Record
	for tick := uint64(1); tick <= 5; tick++ {
		want = append(want, recordsOfEveryKind(log, engine, tick)...)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := readLog(t, dir, engine, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf(
			"got %d records and want %d; first difference:\n%s",
			len(got),
			len(want),
			firstDifference(got, want),
		)
	}
}

func firstDifference(got, want []Record) string {
	for i := range min(len(got), len(want)) {
		if !reflect.DeepEqual(got[i], want[i]) {
			return fmt.Sprintf("got  %+v\nwant %+v", got[i], want[i])
		}
	}
	return "one is a prefix of the other"
}

func TestAnEngineWritesOnlyToItsOwnLogAndTheMetaSaysWhichAndTheSeed(t *testing.T) {
	dir := t.TempDir()
	log := NewFileLog(dir, testFileLogOptions)
	a, b := sim.StateID(1), sim.StateID(21)
	recordsOfEveryKind(log, a, 1)
	recordsOfEveryKind(log, b, 1)
	log.Close()
	for _, e := range []sim.StateID{a, b} {
		got, err := readLog(t, dir, e, 0)
		if err != nil || len(got) != 8 {
			t.Fatalf("%v: %d records, error %v", e, len(got), err)
		}
		for _, r := range got {
			if r.Engine != e {
				t.Fatalf("a record of %v in the log of %v", r.Engine, e)
			}
		}
	}
	for _, name := range []string{"toy-1", "toy-21"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("no directory %s: %v", name, err)
		}
	}
	r, _ := ReadFileLog(dir, a, 0, toyCodecs())
	defer r.Close()
	if !r.Next() || r.Seed() != 42 {
		t.Fatalf("seed %d", r.Seed())
	}
}

func TestReadingFromATickGivesThatTickAndLaterAndUsesTheMetaToStartInTheRightSegment(t *testing.T) {
	dir := t.TempDir()
	o := testFileLogOptions
	o.SegmentBytes = 1 // a segment for each tick
	log := NewFileLog(dir, o)
	engine := sim.StateID(11)
	for tick := uint64(1); tick <= 20; tick++ {
		recordsOfEveryKind(log, engine, tick)
	}
	log.Close()
	segs, _ := segmentlog.Segments(engineDir(dir, engine))
	if len(segs) < 10 {
		t.Fatalf("%d segments, want one for each tick or nearly", len(segs))
	}
	got, err := readLog(t, dir, engine, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 9*8 || got[0].Tick != 12 || got[len(got)-1].Tick != 20 {
		t.Fatalf(
			"%d records from %d to %d, want 72 from 12 to 20",
			len(got),
			got[0].Tick,
			got[len(got)-1].Tick,
		)
	}
}

func TestACrashThatCutsTheEndOfTheFileLeavesTheRecordsBeforeIt(t *testing.T) {
	dir := t.TempDir()
	log := NewFileLog(dir, testFileLogOptions)
	engine := sim.StateID(1)
	for tick := uint64(1); tick <= 3; tick++ {
		recordsOfEveryKind(log, engine, tick)
	}
	log.Close()
	segs, _ := segmentlog.Segments(engineDir(dir, engine))
	st, _ := os.Stat(segs[0].Path)
	os.Truncate(segs[0].Path, st.Size()-3)
	got, err := readLog(t, dir, engine, 0)
	if err != nil {
		t.Fatalf("a torn end is not an error: %v", err)
	}
	if len(got) != 2*8 {
		t.Fatalf("%d records, want the two whole ticks and nothing of the cut one", len(got))
	}
}

func TestARecordThatCannotBeWrittenMarksTheLogAsFailedAndTheOthersAreStillWritten(t *testing.T) {
	dir := t.TempDir()
	log := NewFileLog(dir, testFileLogOptions)
	engine := sim.StateID(1)
	log.Append(engine, 1, ConfigRead{Version: 1})
	log.Append(engine, 1, Command{}) // no command: it cannot be written
	log.Append(engine, 1, ConfigRead{Version: 2})
	log.EndTick(engine, 1)
	if log.Err() == nil {
		t.Fatal("the log was not marked as failed")
	}
	if err := log.Close(); err == nil {
		t.Fatal("Close did not return the error")
	}
	got, err := readLog(t, dir, engine, 0)
	if err != nil || len(got) != 2 || got[0].Body != (ConfigRead{Version: 1}) ||
		got[1].Body != (ConfigRead{Version: 2}) {
		t.Fatalf("%d records, error %v; want the two that could be written", len(got), err)
	}
}

func TestATickWithNoRecordIsAFrameWithOnlyItsTick(t *testing.T) {
	dir := t.TempDir()
	engine := sim.StateID(21)
	log := NewFileLog(dir, testFileLogOptions)
	log.EndTick(engine, 1)
	log.Append(engine, 2, ConfigRead{Version: 3})
	log.EndTick(engine, 2)
	log.EndTick(engine, 3)
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := readLog(t, dir, engine, 0)
	if err != nil || len(got) != 1 || got[0].Tick != 2 || got[0].Body != (ConfigRead{Version: 3}) {
		t.Fatalf("records %v, error %v; want the one of tick 2", got, err)
	}
	r, err := segmentlog.OpenReader(engineDir(dir, engine), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var ticks []uint64
	for r.Next() {
		ticks = append(ticks, r.ID())
	}
	if !reflect.DeepEqual(ticks, []uint64{1, 2, 3}) {
		t.Fatalf("the frames are of the ticks %v, want one for each of 1, 2 and 3", ticks)
	}
}

func TestRecordsOfAnotherTickAreAnErrorAndAreNotWritten(t *testing.T) {
	dir := t.TempDir()
	engine := sim.StateID(21)
	log := NewFileLog(dir, testFileLogOptions)
	log.Append(engine, 1, ConfigRead{Version: 1})
	log.Append(engine, 2, ConfigRead{Version: 2}) // not the tick of the frame
	if log.Err() == nil {
		t.Fatal("a record of another tick was not refused")
	}
	log.EndTick(engine, 1)
	log.Close()
	got, _ := readLog(t, dir, engine, 0)
	if len(got) != 1 || got[0].Tick != 1 {
		t.Fatalf("records %v, want only the one of tick 1", got)
	}
}

func TestTheLogOfAnotherEngineIsRefused(t *testing.T) {
	dir := t.TempDir()
	log := NewFileLog(dir, testFileLogOptions)
	recordsOfEveryKind(log, sim.StateID(11), 1)
	log.Close()
	os.Rename(engineDir(dir, sim.StateID(11)), engineDir(dir, sim.StateID(12)))
	if _, err := readLog(t, dir, sim.StateID(12), 0); err == nil {
		t.Fatal("the log of engine 11 was read as the log of engine 12")
	}
}

func TestACorruptRecordPayloadIsAnErrorAndNeverAPanic(t *testing.T) {
	engine := sim.StateID(1)
	for _, p := range [][]byte{
		nil,
		{},
		{99},
		{byte(KindInbound)},
		{byte(KindInbound), 3, 1, 1, 2, 5},
		{byte(KindReturned), 1},
		{byte(KindHash), 1, 2},
		{byte(KindCommand), 0xff, 0xff, 0x7f},
		{byte(KindConfig), 1, 9},
	} {
		if _, err := decodeRecord(toyCodecs(), engine, formatVersion, 1, p); err == nil {
			t.Errorf("%v was read", p)
		}
	}
}

func TestALogThatCouldNotBeOpenedFailsOnceAndTheEngineDoesNotTryAgainForEachRecord(t *testing.T) {
	dir := t.TempDir()
	engine := sim.StateID(21)
	os.WriteFile(
		engineDir(dir, engine), []byte("a file where the directory of the log should be"), 0o644,
	)
	log := NewFileLog(dir, testFileLogOptions)
	log.Append(engine, 1, ConfigRead{Version: 1})
	if log.Err() == nil {
		t.Fatal("a log that could not be opened is not marked as failed")
	}
	// the obstacle goes away: an engine that tried again for each record would now open its log
	os.Remove(engineDir(dir, engine))
	for tick := uint64(2); tick < 10; tick++ {
		log.Append(engine, tick, ConfigRead{Version: 1})
		log.EndTick(engine, tick)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("the engine opened its log after it had failed: %d entries", len(entries))
	}
	if err := log.Close(); err == nil {
		t.Fatal("Close did not return the error")
	}
}

func TestDurableIsTheTickOfTheLastFrameThatIsOnTheDisk(t *testing.T) {
	dir := t.TempDir()
	engine := sim.StateID(21)
	o := testFileLogOptions
	o.SyncEvery = 5 * time.Millisecond
	log := NewFileLog(dir, o)
	if got := log.Durable(engine); got != 0 {
		t.Fatalf("Durable of an engine with no log = %d", got)
	}
	for tick := uint64(1); tick <= 7; tick++ {
		log.Append(engine, tick, ConfigRead{Version: tick})
		log.EndTick(engine, tick)
		if got := log.Durable(engine); got > tick {
			t.Fatalf("Durable = %d at the tick %d", got, tick)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for log.Durable(engine) != 7 {
		if time.Now().After(deadline) {
			t.Fatalf("Durable = %d after a while, want 7", log.Durable(engine))
		}
		time.Sleep(time.Millisecond)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	// another process opens the log: what is there is on the disk
	again := NewFileLog(dir, o)
	defer again.Close()
	if got := again.Durable(engine); got != 7 {
		t.Fatalf("Durable of a log that was there = %d, want 7", got)
	}
}

// settled waits until what the log says is safe for the engine is the tick, or fails the test.
func settled(t *testing.T, log *FileLog, engine sim.StateID, tick uint64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for log.Safe(engine) != tick {
		if time.Now().After(deadline) {
			t.Fatalf("Safe(%v) = %d after a while, want %d", engine, log.Safe(engine), tick)
		}
		time.Sleep(time.Millisecond)
	}
}

func aMessageFrom(from sim.StateID, seq, tick uint64) sim.Message {
	return benchMessage(1).Stamp(sim.Header{From: from, Seq: seq, Tick: tick})
}

func TestATickThatTookAMessageIsNotSafeUntilTheTickOfItsSenderIs(t *testing.T) {
	dir := t.TempDir()
	a, b := sim.StateID(1), sim.StateID(2)
	quick := testFileLogOptions
	quick.SyncEvery = 2 * time.Millisecond
	log := NewFileLog(dir, quick)
	defer log.Close()
	// b takes at its tick 2 the message that a sent at its tick 3; b syncs often and a does not
	// write at all yet
	log.Append(b, 1, ConfigRead{Version: 1})
	log.EndTick(b, 1)
	log.Append(b, 2, Inbound{Message: aMessageFrom(a, 1, 3)})
	log.EndTick(b, 2)
	log.EndTick(b, 3)
	if got := log.Durable(b); got > 3 {
		t.Fatalf("Durable(b) = %d", got)
	}
	settled(t, log, b, 1) // b is durable up to 3, and safe only before the tick that depends on a
	// a has written and synced its ticks 1 to 3: b is safe up to its own durable tick
	for tick := uint64(1); tick <= 3; tick++ {
		log.EndTick(a, tick)
	}
	settled(t, log, a, 3)
	settled(t, log, b, 3)
}

func TestWhatIsSafeWaitsAcrossAChainOfDependencies(t *testing.T) {
	dir := t.TempDir()
	a, b, c := sim.StateID(1), sim.StateID(2), sim.StateID(3)
	quick := testFileLogOptions
	quick.SyncEvery = 2 * time.Millisecond
	log := NewFileLog(dir, quick)
	defer log.Close()
	// c took a message of b from b's tick 2, and b took at its tick 2 a copy of a that a took at 4
	log.Append(c, 1, Inbound{Message: aMessageFrom(b, 1, 2)})
	log.EndTick(c, 1)
	log.EndTick(b, 1)
	log.Append(b, 2, StableRead{Source: a, CopyTick: 4})
	log.EndTick(b, 2)
	settled(t, log, c, 0)
	settled(t, log, b, 1)
	for tick := uint64(1); tick <= 3; tick++ {
		log.EndTick(a, tick)
	}
	settled(t, log, a, 3)
	settled(t, log, b, 1) // a is safe only up to 3: b waits for its tick 4
	log.EndTick(a, 4)
	settled(t, log, a, 4)
	settled(t, log, b, 2)
	settled(t, log, c, 1) // c was durable up to 1, and its message was b's at tick 2, which is safe
}

func TestAnEngineWhoseLogWasThereBeforeIsSafeUpToItsLastTick(t *testing.T) {
	dir := t.TempDir()
	engine := sim.StateID(1)
	first := NewFileLog(dir, testFileLogOptions)
	for tick := uint64(1); tick <= 5; tick++ {
		first.EndTick(engine, tick)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second := NewFileLog(dir, testFileLogOptions)
	defer second.Close()
	if got := second.Safe(engine); got != 5 {
		t.Fatalf("Safe = %d, want 5: what is in the log is what a restart goes on from", got)
	}
}

func TestACloseSettlesWhatIsSafeOfEveryEngine(t *testing.T) {
	dir := t.TempDir()
	a, b := sim.StateID(1), sim.StateID(2)
	log := NewFileLog(dir, testFileLogOptions) // a sync every hour: only Close syncs
	log.EndTick(a, 1)
	log.Append(b, 1, Inbound{Message: aMessageFrom(a, 1, 1)})
	log.EndTick(b, 1)
	if log.Safe(b) != 0 {
		t.Fatalf("Safe(b) = %d before any sync", log.Safe(b))
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	if log.Safe(a) != 1 || log.Safe(b) != 1 {
		t.Fatalf("after Close Safe(a) = %d, Safe(b) = %d, want 1, 1", log.Safe(a), log.Safe(b))
	}
}

func TestAFrameMustFollowTheLastOfTheLog(t *testing.T) {
	dir := t.TempDir()
	engine := sim.StateID(21)
	log := NewFileLog(dir, testFileLogOptions)
	log.Append(engine, 1, ConfigRead{Version: 1})
	log.EndTick(engine, 1)
	log.EndTick(engine, 2)
	if log.Err() != nil {
		t.Fatalf("error %v", log.Err())
	}
	log.Append(engine, 4, ConfigRead{Version: 2})
	log.EndTick(engine, 4) // not the tick after 2: it is not written
	if log.Err() == nil {
		t.Fatal("a frame that does not follow was accepted")
	}
	log.Close()
	if last, err := segmentlog.LastFrameID(engineDir(dir, engine)); err != nil || last != 2 {
		t.Fatalf("the last frame of the log is %d, error %v, want 2", last, err)
	}
	// a log that was there is written on after its last frame
	again := NewFileLog(dir, testFileLogOptions)
	again.EndTick(engine, 3)
	if err := again.Close(); err != nil {
		t.Fatalf("the log that was there could not be written on: %v", err)
	}
	if last, err := segmentlog.LastFrameID(engineDir(dir, engine)); err != nil || last != 3 {
		t.Fatalf("the last frame of the log is %d, error %v, want 3", last, err)
	}
}

func TestTheFirstFrameOfALogIsTheTickOne(t *testing.T) {
	dir := t.TempDir()
	engine := sim.StateID(21)
	log := NewFileLog(dir, testFileLogOptions)
	log.Append(engine, 4, ConfigRead{Version: 1})
	log.EndTick(engine, 4) // a log has the tick 1 as its first
	if log.Err() == nil {
		t.Fatal("a first frame of the tick 4 was accepted")
	}
	log.Close()
	if last, err := segmentlog.LastFrameID(engineDir(dir, engine)); err != nil || last != 0 {
		t.Fatalf("the last frame of the log is %d, error %v, want none", last, err)
	}
}

// safeCalls records what a FileLog tells its observer.
type safeCalls struct {
	mu    sync.Mutex
	calls []map[sim.StateID]uint64
	final []bool
}

func (s *safeCalls) SafeChanged(safe map[sim.StateID]uint64, final bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, safe)
	s.final = append(s.final, final)
}

func TestTheObserverOfALogIsToldWhatIsSafeOfEveryEngineAndOnceMoreWhenTheLogIsClosed(t *testing.T) {
	dir := t.TempDir()
	observer := &safeCalls{}
	opts := testFileLogOptions
	opts.OnSafe = observer
	log := NewFileLog(dir, opts)
	a, b := sim.StateID(1), sim.StateID(2)
	for tick := uint64(1); tick <= 4; tick++ {
		log.EndTick(a, tick)
		log.EndTick(b, tick)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	observer.mu.Lock()
	defer observer.mu.Unlock()
	n := len(observer.calls)
	if n == 0 || !observer.final[n-1] {
		t.Fatalf("%d calls, the last final: %v", n, observer.final)
	}
	if last := observer.calls[n-1]; !reflect.DeepEqual(last, map[sim.StateID]uint64{a: 4, b: 4}) {
		t.Fatalf("the last call says %v, want both at 4", last)
	}
	for i := 0; i < n-1; i++ {
		if observer.final[i] {
			t.Fatalf("the call %d is final, and is not the last", i)
		}
	}
}
