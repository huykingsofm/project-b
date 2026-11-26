package game_test

import (
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game"
	"github.com/huykingsofm/project-b/server/game/testkit"
	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// handedOver is a recovery of the files under dir and the cluster that goes on from it, whose event
// log writes on after the cut. closeLog closes that log, so that the files can be recovered again.
func handedOver(
	t *testing.T, dir string,
) (result *replay.Result, live *engine.Cluster, closeLog func()) {
	t.Helper()
	return handedOverAs(t, dir, func(b *game.ClusterBuilder) replay.Builder { return b })
}

// handedOverAs is handedOver with the builder that the handover is given made from the builder of
// the world by as.
func handedOverAs(
	t *testing.T, dir string, as func(*game.ClusterBuilder) replay.Builder,
) (result *replay.Result, live *engine.Cluster, closeLog func()) {
	t.Helper()
	result = recoverTheWorld(t, dir, everySeventhTick)
	file := eventlog.NewFileLog(dir, eventlog.FileLogOptions{
		Seed: 7, SegmentBytes: 2048, SyncEvery: time.Hour, Backlog: 8,
	})
	builder := testkit.WorldBuilder(t, 7, 10).WithEventLog(file)
	env, err := builder.NewEnv()
	if err != nil {
		t.Fatal(err)
	}
	if live, err = result.Handover(as(builder), env); err != nil {
		t.Fatal(err)
	}
	return result, live, func() {
		t.Helper()
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEveryEngineThatTheHandoverMakesHasTheTickAndTheHashOfTheOneThatWasReplayed(t *testing.T) {
	dir := t.TempDir()
	aWorldThatWroteItsFilesAndStopped(t, dir, everySeventhTick, 0)
	result, live, closeLog := handedOver(t, dir)
	defer closeLog()
	if got, want := len(live.Engines()), len(result.Cluster.Engines()); got != want {
		t.Fatalf("%d engines were made, %d were replayed", got, want)
	}
	for _, replayed := range result.Cluster.Engines() {
		id := replayed.StateID()
		e, ok := live.Find(id)
		if !ok {
			t.Fatalf("%v was not made", id)
		}
		if e.Tick() != replayed.Tick() {
			t.Fatalf("%v is at tick %d, was replayed to %d", id, e.Tick(), replayed.Tick())
		}
		if got, want := e.Hash(sim.NewHasher()), replayed.Hash(sim.NewHasher()); got != want {
			t.Fatalf("%v has the hash %d, was replayed to %d", id, got, want)
		}
	}
}

func TestTheLogsThatAHandedOverClusterWritesAreRecoveredWithNoDivergence(t *testing.T) {
	dir := t.TempDir()
	aWorldThatWroteItsFilesAndStopped(t, dir, everySeventhTick, 0)
	first, live, closeLog := handedOver(t, dir)
	const ticks = 40
	for range ticks {
		if err := live.Step(); err != nil {
			t.Fatal(err)
		}
	}
	closeLog()
	second := recoverTheWorld(t, dir, everySeventhTick)
	for _, id := range testkit.ClusterEngines {
		if want := first.Ticks[id] + ticks; second.Ticks[id] != want {
			t.Fatalf("%v was recovered to the tick %d, want %d", id, second.Ticks[id], want)
		}
	}
}

// takenMessages are the messages that the log of the engine says it took, and how many times.
func takenMessages(t *testing.T, dir string, id sim.StateID) map[[2]uint64]int {
	t.Helper()
	logs, err := eventlog.NewFileLogs(dir, testkit.WorldBuilder(t, 7, 10).Codecs())
	if err != nil {
		t.Fatal(err)
	}
	src, err := logs.Open(id, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	taken := map[[2]uint64]int{}
	for src.Next() {
		if in, ok := src.Record().Body.(eventlog.Inbound); ok {
			h := in.Message.Head()
			taken[[2]uint64{uint64(h.From), h.Seq}]++
		}
	}
	if err := src.Err(); err != nil {
		t.Fatal(err)
	}
	return taken
}

func TestAMessageThatItsReceiverLostIsTakenOnceAfterTheHandoverAndOneItTookIsNot(t *testing.T) {
	dir := t.TempDir()
	aWorldThatWroteItsFilesAndStopped(t, dir, everyFiveHundredTicks, 0)
	// no engine has a checkpoint, so every message is one that the replay made again
	// a crash: the second zonelayer lost its log after tick 7, and the others kept theirs
	const kept = 7
	if err := segmentlog.CutAfter(
		eventlog.EngineDir(dir, testkit.Layer2), kept, t.TempDir(),
	); err != nil {
		t.Fatal(err)
	}
	_, live, closeLog := handedOver(t, dir)
	tookBefore := takenMessages(t, dir, testkit.Layer2)
	var lost [][2]uint64
	for _, e := range live.Engines() {
		for _, m := range e.SentMessages() {
			h := m.Head()
			key := [2]uint64{uint64(h.From), h.Seq}
			if m.Dest() == testkit.Layer2 && tookBefore[key] == 0 {
				lost = append(lost, key)
			}
		}
	}
	if len(lost) == 0 {
		t.Fatal("no message to the second zonelayer was lost: the test has nothing to show")
	}
	for range 40 {
		if err := live.Step(); err != nil {
			t.Fatal(err)
		}
	}
	closeLog()
	after := takenMessages(t, dir, testkit.Layer2)
	for _, key := range lost {
		if after[key] != 1 {
			t.Fatalf("the message %v was taken %d times, want 1", key, after[key])
		}
	}
	for key := range tookBefore {
		if after[key] != 1 {
			t.Fatalf("the message %v was taken %d times, want 1", key, after[key])
		}
	}
	recoverTheWorld(t, dir, everySeventhTick)
}
