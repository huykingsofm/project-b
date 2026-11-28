package game_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/command"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/testkit"
)

// recordsOf returns the records of the memory log that belong to an engine, in order.
func recordsOf(mem *eventlog.MemLog, e sim.StateID) []eventlog.Record {
	var out []eventlog.Record
	for _, r := range mem.Records {
		if r.Engine != e {
			continue
		}
		out = append(out, r)
	}
	return out
}

// compareWithFiles reads the log of every engine and checks that it is the memory log of that
// engine.
func compareWithFiles(
	t *testing.T, dir string, mem *eventlog.MemLog, codecs *sim.Codecs,
) {
	t.Helper()
	kinds := map[eventlog.Kind]int{}
	for _, e := range testkit.ClusterEngines {
		want := recordsOf(mem, e)
		r, err := eventlog.ReadFileLog(dir, e, 0, codecs)
		if err != nil {
			t.Fatal(err)
		}
		var got []eventlog.Record
		for r.Next() {
			got = append(got, r.Record())
		}
		if err := r.Err(); err != nil {
			t.Fatalf("%v: %v", e, err)
		}
		r.Close()
		if len(want) == 0 {
			t.Fatalf("%v recorded nothing: the scenario does not reach it", e)
		}
		if !reflect.DeepEqual(got, want) {
			for i := range min(len(got), len(want)) {
				if !reflect.DeepEqual(got[i], want[i]) {
					t.Fatalf(
						"%v: record %d differs:\n log    %+v\n memory %+v", e, i, got[i], want[i],
					)
				}
			}
			t.Fatalf("%v: the log has %d records and memory has %d", e, len(got), len(want))
		}
		for _, rec := range got {
			kinds[rec.Kind()]++
		}
	}
	for _, k := range []eventlog.Kind{
		eventlog.KindInbound,
		eventlog.KindCommand,
		eventlog.KindHash,
		eventlog.KindStableRead,
	} {
		if kinds[k] == 0 {
			t.Errorf("no record of kind %d in the scenario", k)
		}
	}
}

func TestTheFilesOfAWholeClusterRunHoldWhatTheMemoryLogHolds(t *testing.T) {
	dir := t.TempDir()
	c, mem, file := testkit.NewTeeWorld(t, dir)
	testkit.RunScenario(t, c, testkit.Sender{T: t})
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	compareWithFiles(t, dir, mem, c.Env.Codecs)
}

// The engines must not depend on memory they share with the sender of a command. The sender here
// goes on using the character it sent and changes its cooldowns; the same run with every command
// written to bytes and read back, which shares nothing with the sender, must give the same hash at
// every tick that records one and the same state at the end. (The memory log keeps a command as the
// sender gave it, so its command records are not compared.)
func TestACommandThatIsCopiedGivesTheSameRunAsOneThatIsNot(t *testing.T) {
	run := func(roundTrip bool) ([]eventlog.Record, []uint64) {
		c, _, mem := testkit.NewWorld(t, 7, 10)
		s := testkit.Sender{T: t, RoundTrip: roundTrip, Reuses: true, Codecs: c.Env.Codecs}
		testkit.RunScenario(t, c, s)
		var hashes []eventlog.Record
		for _, r := range mem.Records {
			if r.Kind() == eventlog.KindHash {
				hashes = append(hashes, r)
			}
		}
		return hashes, []uint64{
			c.World.State().Hash(),
			c.Zone(testkit.Zone1).State().Hash(), c.Zone(testkit.Zone2).State().Hash(),
			c.ZoneLayer(testkit.Layer1).State().Hash(), c.ZoneLayer(testkit.Layer2).State().Hash(),
			c.ZoneLayer(testkit.Layer3).State().Hash(),
		}
	}
	directHashes, directFinal := run(false)
	copiedHashes, copiedFinal := run(true)
	if len(directHashes) == 0 {
		t.Fatal("the scenario recorded no hash")
	}
	if !reflect.DeepEqual(directHashes, copiedHashes) {
		t.Fatalf("the hashes recorded differ:\n direct %v\n copied %v", directHashes, copiedHashes)
	}
	if !reflect.DeepEqual(directFinal, copiedFinal) {
		t.Fatalf("the state at the end differs: %v and %v", directFinal, copiedFinal)
	}
}

func TestTheFilesOfEnginesThatRunOnTheirOwnGoroutinesHoldWhatTheMemoryLogHolds(t *testing.T) {
	dir := t.TempDir()
	c, mem, file := testkit.NewTeeWorld(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		if err := c.Run(ctx, time.Millisecond); err != nil {
			t.Error(err)
		}
		close(done)
	}()

	for i := range 30 {
		c.Zone(testkit.Zone1).SendCommand(
			command.ZoneCharacterEnterCommand{
				Character: testkit.NewCharacter(state.CharacterID(i+1), "x", 100, 1000, 0),
			},
		)
		c.World.SendCommand(command.WorldChatPrivateSendCommand{FromID: 1, ToID: 2, Text: "psst"})
		time.Sleep(2 * time.Millisecond)
	}
	time.Sleep(60 * time.Millisecond)
	cancel()
	<-done
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	for _, e := range []sim.StateID{testkit.World, testkit.Zone1, testkit.Layer1} {
		want := recordsOf(mem, e)
		r, err := eventlog.ReadFileLog(dir, e, 0, c.Env.Codecs)
		if err != nil {
			t.Fatal(err)
		}
		var got []eventlog.Record
		for r.Next() {
			got = append(got, r.Record())
		}
		if r.Err() != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf(
				"%v: %d records in the log, %d in memory, error %v",
				e,
				len(got),
				len(want),
				r.Err(),
			)
		}
		r.Close()
	}
}
