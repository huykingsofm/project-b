package replay

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// ticks writes the ticks from to to of an engine, each with the records that at says (none for a
// tick that is not in it), and closes the log of the cluster at the end of the test.
func ticks(log *FileLog, engine sim.StateID, to uint64, at map[uint64]Body) {
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
		last, err := segmentlog.LastFrameID(EngineDir(dir, e))
		if err != nil {
			t.Fatal(err)
		}
		out[e] = last
	}
	return out
}

func TestTheRecoveryLineCutsAnEngineBeforeATickThatDependsOnATickThatWasLost(t *testing.T) {
	dir := t.TempDir()
	a, b, c := sim.StateID(1), sim.StateID(2), sim.StateID(3)
	d, lone := sim.StateID(4), sim.StateID(8)
	log := NewFileLog(dir, testFileLogOptions)
	// a was lost after its tick 5; b took at its tick 4 a message that a sent at 6; c took at its
	// tick 5 a message that b sent at 5, which b lost; d read at 2 a copy of an engine that has no
	// log; lone depends on nothing
	ticks(log, a, 5, nil)
	ticks(log, b, 6, map[uint64]Body{4: Inbound{Message: aMessageFrom(a, 1, 6)}})
	ticks(log, c, 7, map[uint64]Body{
		5: Inbound{Message: aMessageFrom(b, 1, 5)},
		2: Inbound{Message: aMessageFrom(a, 2, 5)}, // a sent it at 5, which is kept
	})
	ticks(log, d, 4, map[uint64]Body{2: StableRead{Source: 99, CopyTick: 1}})
	ticks(log, lone, 9, nil)
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := RecoveryLine(dir, toyCodecs())
	if err != nil {
		t.Fatal(err)
	}
	want := map[sim.StateID]uint64{a: 5, b: 3, c: 4, d: 1, lone: 9}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the recovery line is %v, want %v", got, want)
	}
}

func TestACutToTheRecoveryLineLeavesLogsThatEndAtThePointAndAreNotChangedByAnother(t *testing.T) {
	dir := t.TempDir()
	a, b := sim.StateID(1), sim.StateID(2)
	log := NewFileLog(dir, testFileLogOptions)
	ticks(log, a, 3, nil)
	ticks(log, b, 8, map[uint64]Body{
		5: Inbound{Message: aMessageFrom(a, 1, 4)}, // a was lost after 3
	})
	log.Close()
	limits, err := CutToRecoveryLine(dir, toyCodecs())
	if err != nil {
		t.Fatal(err)
	}
	want := map[sim.StateID]uint64{a: 3, b: 4}
	if !reflect.DeepEqual(limits, want) || !reflect.DeepEqual(lastTicksOf(t, dir, a, b), want) {
		t.Fatalf("limits %v, last ticks %v, want %v", limits, lastTicksOf(t, dir, a, b), want)
	}
	discarded, _ := filepath.Glob(filepath.Join(engineDir(dir, b), "*.discarded-*"))
	if len(discarded) == 0 {
		t.Fatal("what was cut of the log of b was not kept")
	}
	again, err := CutToRecoveryLine(dir, toyCodecs())
	if err != nil || !reflect.DeepEqual(again, want) {
		t.Fatalf("a second cut gave %v, %v", again, err)
	}
	// the log that was cut is written on after its last tick
	reopened := NewFileLog(dir, testFileLogOptions)
	defer reopened.Close()
	reopened.EndTick(b, 5)
	if reopened.Err() != nil {
		t.Fatalf("the log that was cut could not be written on: %v", reopened.Err())
	}
}
