package engine_test

import (
	"context"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/enginetest"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"reflect"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/command"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
)

// recordsOf returns the records of the memory log that belong to an engine, in order.
func recordsOf(mem *replay.MemLog, e message.Address) []replay.Record {
	var out []replay.Record
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
func compareWithFiles(t *testing.T, dir string, mem *replay.MemLog) {
	t.Helper()
	kinds := map[replay.Kind]int{}
	for _, e := range enginetest.ClusterEngines {
		want := recordsOf(mem, e)
		r, err := replay.ReadFileLog(dir, e, 0)
		if err != nil {
			t.Fatal(err)
		}
		var got []replay.Record
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
	for _, k := range []replay.Kind{
		replay.KindInbound,
		replay.KindCommand,
		replay.KindConfig,
		replay.KindHash,
		replay.KindStableRead,
	} {
		if kinds[k] == 0 {
			t.Errorf("no record of kind %d in the scenario", k)
		}
	}
}

func TestTheFilesOfAWholeClusterRunHoldWhatTheMemoryLogHolds(t *testing.T) {
	dir := t.TempDir()
	c, mem, file, cfg := enginetest.NewTeeWorld(t, dir)
	enginetest.RunScenario(t, c, cfg, enginetest.Sender{T: t})
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	compareWithFiles(t, dir, mem)
}

// The engines must not depend on memory they share with the sender of a command. The sender here
// goes on using the character it sent and changes its cooldowns; the same run with every command
// written to bytes and read back, which shares nothing with the sender, must give the same hash at
// every tick that records one and the same state at the end. (The memory log keeps a command as the
// sender gave it, so its command records are not compared.)
func TestACommandThatIsCopiedGivesTheSameRunAsOneThatIsNot(t *testing.T) {
	run := func(roundTrip bool) ([]replay.Record, []uint64) {
		c, _, mem, cfg := enginetest.NewWorld(t, 7, 10)
		s := enginetest.Sender{T: t, RoundTrip: roundTrip, Reuses: true}
		enginetest.RunScenario(t, c, cfg, s)
		var hashes []replay.Record
		for _, r := range mem.Records {
			if r.Kind() == replay.KindHash {
				hashes = append(hashes, r)
			}
		}
		return hashes, []uint64{
			c.World.State().Hash(), c.Zone(1).State().Hash(), c.Zone(2).State().Hash(),
			c.ZoneLayer(1).State().Hash(), c.ZoneLayer(2).State().Hash(), c.ZoneLayer(
				3,
			).State().Hash(),
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
	c, mem, file, _ := enginetest.NewTeeWorld(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx, time.Millisecond); close(done) }()

	for i := range 30 {
		c.Zone(1).Send(
			command.ZoneCharacterEnterCommand{
				Character: enginetest.NewCharacter(state.CharacterID(i+1), "x", 100, 1000, 0),
			},
		)
		c.World.Send(command.WorldChatPrivateSendCommand{FromID: 1, ToID: 2, Text: "psst"})
		time.Sleep(2 * time.Millisecond)
	}
	time.Sleep(60 * time.Millisecond)
	cancel()
	<-done
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	for _, e := range []message.Address{
		message.WorldAddress(1), message.ZoneAddress(1), message.ZoneLayerAddress(1),
	} {
		want := recordsOf(mem, e)
		r, err := replay.ReadFileLog(dir, e, 0)
		if err != nil {
			t.Fatal(err)
		}
		var got []replay.Record
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
