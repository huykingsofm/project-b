package replay

import (
	"fmt"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/onfail"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/command"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/internal/shared/segmentlog"
)

var testFileLogOptions = FileLogOptions{
	Seed: 42, SegmentBytes: 1 << 20, SyncEvery: time.Hour, Backlog: 4,
}

func readLog(t *testing.T, dir string, engine message.Address, from uint64) ([]Record, error) {
	t.Helper()
	r, err := ReadFileLog(dir, engine, from)
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
func recordsOfEveryKind(log Log, engine message.Address, tick uint64) []Record {
	from := message.ZoneLayerAddress(3)
	inbound := action.MessageToZoneLayer(
		3, onfail.Compensate{With: action.MessageToZoneLayer(
			1, onfail.Drop{}, &action.ZoneLayerCharacterRegenAction{},
		)},
		&action.ZoneLayerCharacterMoveAction{CharacterID: 7, Dir: state.East, Ticks: 3},
		&action.ZoneLayerChatSendAction{FromID: 7, Text: "xin chào"},
	).Stamp(message.Header{From: from, Seq: tick * 10})
	ch := zonelayerstate.Character{
		CharacterID: 7, Name: "Binh", Health: 80, MaxHealth: 100, Radius: 256, Speed: 250,
	}
	ch.Cooldowns.Set(action.ZoneLayerCharacterNormalAttackTriggerActionCooldownKey, tick+6)
	enter := command.ZoneCharacterEnterCommand{Character: ch}
	move := command.ZoneLayerCharacterMoveCommand{CharacterID: 7, Dir: state.East, Ticks: 3}
	chat := command.WorldChatPrivateSendCommand{FromID: 1, ToID: 2, Text: "psst"}

	log.Append(engine, tick, ConfigRead{Version: 5})
	log.Append(engine, tick, Inbound{Message: inbound})
	log.Append(engine, tick, Returned{
		Header: message.Header{From: from, Seq: tick*10 + 1}, Reason: message.Full,
	})
	log.Append(engine, tick, Command{Command: chat})
	log.Append(engine, tick, Command{Command: enter})
	log.Append(engine, tick, Command{Command: move})
	log.Append(engine, tick, StableRead{Source: message.ZoneAddress(2), CopyTick: tick - 1})
	log.Append(engine, tick, Hash{Value: 0xdeadbeefcafef00d + tick})
	log.EndTick(engine, tick)
	return []Record{
		{Engine: engine, Tick: tick, Body: ConfigRead{Version: 5}},
		{Engine: engine, Tick: tick, Body: Inbound{Message: inbound}},
		{
			Engine: engine,
			Tick:   tick,
			Body: Returned{
				Header: message.Header{From: from, Seq: tick*10 + 1}, Reason: message.Full,
			},
		},
		{Engine: engine, Tick: tick, Body: Command{Command: chat}},
		{Engine: engine, Tick: tick, Body: Command{Command: enter}},
		{Engine: engine, Tick: tick, Body: Command{Command: move}},
		{
			Engine: engine,
			Tick:   tick,
			Body:   StableRead{Source: message.ZoneAddress(2), CopyTick: tick - 1},
		},
		{Engine: engine, Tick: tick, Body: Hash{Value: 0xdeadbeefcafef00d + tick}},
	}
}

func TestEveryKindOfRecordComesBackAsItWasWritten(t *testing.T) {
	dir := t.TempDir()
	log := NewFileLog(dir, testFileLogOptions)
	engine := message.ZoneLayerAddress(1)
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
	a, b := message.ZoneLayerAddress(1), message.WorldAddress(1)
	recordsOfEveryKind(log, a, 1)
	recordsOfEveryKind(log, b, 1)
	log.Close()
	for _, e := range []message.Address{a, b} {
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
	for _, name := range []string{"zonelayer-1", "world-1"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("no directory %s: %v", name, err)
		}
	}
	r, _ := ReadFileLog(dir, a, 0)
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
	engine := message.ZoneAddress(1)
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
	engine := message.ZoneLayerAddress(1)
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
	engine := message.ZoneLayerAddress(1)
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

func TestAnEndOfTickBeforeAnyRecordCreatesNothing(t *testing.T) {
	dir := t.TempDir()
	log := NewFileLog(dir, testFileLogOptions)
	log.EndTick(message.WorldAddress(1), 1)
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("%d entries in the directory", len(entries))
	}
}

func TestTheLogOfAnotherEngineIsRefused(t *testing.T) {
	dir := t.TempDir()
	log := NewFileLog(dir, testFileLogOptions)
	recordsOfEveryKind(log, message.ZoneAddress(1), 1)
	log.Close()
	os.Rename(engineDir(dir, message.ZoneAddress(1)), engineDir(dir, message.ZoneAddress(2)))
	if _, err := readLog(t, dir, message.ZoneAddress(2), 0); err == nil {
		t.Fatal("the log of zone 1 was read as the log of zone 2")
	}
}

func TestACorruptRecordPayloadIsAnErrorAndNeverAPanic(t *testing.T) {
	engine := message.ZoneLayerAddress(1)
	for _, p := range [][]byte{
		nil,
		{},
		{99},
		{byte(KindInbound)},
		{byte(KindInbound), 1, 3, 1, 1, 2, 5},
		{byte(KindReturned), 1, 1},
		{byte(KindHash), 1, 1, 2},
		{byte(KindCommand), 1, 0xff, 0xff, 0x7f},
		{byte(KindConfig), 1, 1, 9},
	} {
		if _, err := decodeRecord(engine, formatVersion, p); err == nil {
			t.Errorf("%v was read", p)
		}
	}
}

func TestALogThatCouldNotBeOpenedFailsOnceAndTheEngineDoesNotTryAgainForEachRecord(t *testing.T) {
	dir := t.TempDir()
	engine := message.WorldAddress(1)
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
