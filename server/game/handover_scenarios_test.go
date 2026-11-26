package game_test

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/command"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/game/testkit"
	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// The scenarios of a handover after a crash: which engine lost its log and from which tick, how
// often the engines took a checkpoint, what the cluster does after the handover. Each ends in the
// same checks: every message that was held and that its receiver had not taken is taken once, no
// log holds a message twice, and the logs that were written after the handover recover again with
// no divergence.

// messagesSentAgain is how many held messages the scenarios of a handover found that their
// receivers had not taken, so that a test can tell that the scenarios reach what they are for.
var messagesSentAgain atomic.Int64

var (
	everyTick = map[string]string{"checkpoint.every_ticks": "1"}
	never     = everyFiveHundredTicks // no engine takes a checkpoint within the scenario
)

// crash cuts the log of the engine to the tick kept, as a crash that lost what followed does.
func crash(t *testing.T, dir string, id sim.StateID, kept uint64) {
	t.Helper()
	if err := segmentlog.CutAfter(eventlog.EngineDir(dir, id), kept, t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

// recordsOnDisk is the records of the log of the engine under dir, in order.
func recordsOnDisk(t *testing.T, dir string, id sim.StateID) []eventlog.Record {
	t.Helper()
	r, err := eventlog.ReadFileLog(dir, id, 0, testkit.WorldBuilder(t, 7, 10).Codecs())
	if err != nil {
		t.Fatal(err)
	}
	var out []eventlog.Record
	for r.Next() {
		out = append(out, r.Record())
	}
	if err := r.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// heldAndNotTaken is the messages that the engines of the cluster hold as sent and that their
// receivers have not taken in the logs under dir.
func heldAndNotTaken(t *testing.T, dir string, live *engine.Cluster) [][3]uint64 {
	t.Helper()
	taken := map[sim.StateID]map[[2]uint64]int{}
	var out [][3]uint64
	for _, e := range live.Engines() {
		for _, m := range e.SentMessages() {
			h := m.Head()
			if taken[m.Dest()] == nil {
				taken[m.Dest()] = takenMessages(t, dir, m.Dest())
			}
			if taken[m.Dest()][[2]uint64{uint64(h.From), h.Seq}] == 0 {
				out = append(out, [3]uint64{uint64(m.Dest()), uint64(h.From), h.Seq})
			}
		}
	}
	return out
}

// everyMessageIsTakenOnce fails the test when a log under dir holds a message twice, or when a
// message in lost is not in the log of its receiver exactly once.
func everyMessageIsTakenOnce(t *testing.T, dir string, lost [][3]uint64) {
	t.Helper()
	for _, id := range testkit.ClusterEngines {
		for key, n := range takenMessages(t, dir, id) {
			if n != 1 {
				t.Fatalf("%v took the message %v %d times", id, key, n)
			}
		}
	}
	for _, key := range lost {
		if n := takenMessages(t, dir, sim.StateID(key[0]))[[2]uint64{key[1], key[2]}]; n != 1 {
			t.Fatalf("the message %v that was lost was taken %d times, want 1", key, n)
		}
	}
}

func step(t *testing.T, c *engine.Cluster, n int) {
	t.Helper()
	for range n {
		if err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
}

// aCrashThenAHandover writes a world with the checkpoints every says, cuts each engine of crashes
// to its tick, hands the cluster over, runs it for n ticks, and checks what every scenario does.
func aCrashThenAHandover(
	t *testing.T, every map[string]string, crashes map[sim.StateID]uint64, n int,
) {
	t.Helper()
	dir := t.TempDir()
	aWorldThatWroteItsFilesAndStopped(t, dir, every, 0)
	for id, kept := range crashes {
		crash(t, dir, id, kept)
	}
	first, live, closeLog := handedOver(t, dir)
	lost := heldAndNotTaken(t, dir, live)
	messagesSentAgain.Add(int64(len(lost)))
	step(t, live, n)
	closeLog()
	everyMessageIsTakenOnce(t, dir, lost)
	second := recoverTheWorld(t, dir, every)
	for _, id := range testkit.ClusterEngines {
		if want := first.Ticks[id] + uint64(n); second.Ticks[id] != want {
			t.Fatalf("%v was recovered to the tick %d, want %d", id, second.Ticks[id], want)
		}
	}
}

func TestAHandoverAfterAnyEngineLostItsLogAtAnyTickKeepsEveryMessageOnce(t *testing.T) {
	messagesSentAgain.Store(0)
	t.Run("cases", func(t *testing.T) { // the parallel cases end before this returns
		for name, every := range map[string]map[string]string{
			"a checkpoint every tick":    everyTick,
			"a checkpoint every 7 ticks": everySeventhTick,
			"no checkpoint at all":       never,
		} {
			for _, id := range testkit.ClusterEngines {
				for _, kept := range []uint64{1, 3, 10, 29, 60} {
					t.Run(fmt.Sprintf("%s/%v/kept %d", name, id, kept), func(t *testing.T) {
						t.Parallel()
						aCrashThenAHandover(t, every, map[sim.StateID]uint64{id: kept}, 30)
					})
				}
			}
		}
	})
	if messagesSentAgain.Load() == 0 {
		t.Fatal("no scenario had a message that its receiver lost: they do not reach their aim")
	}
}

func TestAHandoverAfterSeveralEnginesLostTheirLogsAtOnce(t *testing.T) {
	for name, crashes := range map[string]map[sim.StateID]uint64{
		"both zonelayers of a zone": {testkit.Layer1: 10, testkit.Layer2: 20},
		"the zones":                 {testkit.Zone1: 15, testkit.Zone2: 5},
		"the world and a layer":     {testkit.World: 12, testkit.Layer3: 30},
		"every engine at its own tick": {
			testkit.World: 9, testkit.Zone1: 19, testkit.Zone2: 29, testkit.Layer1: 39,
			testkit.Layer2: 49, testkit.Layer3: 59,
		},
		"every engine at the first tick": {
			testkit.World: 1, testkit.Zone1: 1, testkit.Zone2: 1, testkit.Layer1: 1,
			testkit.Layer2: 1, testkit.Layer3: 1,
		},
	} {
		for _, every := range []map[string]string{everySeventhTick, never} {
			t.Run(fmt.Sprintf("%s/%v", name, every), func(t *testing.T) {
				t.Parallel()
				aCrashThenAHandover(t, every, crashes, 30)
			})
		}
	}
}

func TestAHandoverOfAnEngineWhoseLogIsGoneStartsItNewAndCutsWhatDependsOnIt(t *testing.T) {
	dir := t.TempDir()
	aWorldThatWroteItsFilesAndStopped(t, dir, everySeventhTick, 0)
	if err := os.RemoveAll(eventlog.EngineDir(dir, testkit.Layer3)); err != nil {
		t.Fatal(err)
	}
	first, live, closeLog := handedOver(t, dir)
	if got := first.Ticks[testkit.Layer3]; got != 0 {
		t.Fatalf("the engine with no log was replayed to %d, want 0", got)
	}
	published := live.Env.Stable.(*engine.StableStateStore)
	for _, e := range live.Engines() {
		copy := published.Copy(0, 0, e.StateID())
		switch {
		case e.Tick() == 0 && copy != nil:
			t.Fatalf("%v ran no tick and has a copy at the tick %d", e.StateID(), copy.Tick())
		case e.Tick() > 0 && (copy == nil || copy.Tick() != e.Tick()):
			t.Fatalf("%v is at the tick %d and has the copy %v", e.StateID(), e.Tick(), copy)
		}
	}
	lost := heldAndNotTaken(t, dir, live)
	step(t, live, 30)
	closeLog()
	everyMessageIsTakenOnce(t, dir, lost)
	recoverTheWorld(t, dir, everySeventhTick)
}

func TestAClusterThatWasHandedOverThreeTimesRecoversEachTime(t *testing.T) {
	dir := t.TempDir()
	aWorldThatWroteItsFilesAndStopped(t, dir, everySeventhTick, 0)
	crash(t, dir, testkit.Layer2, 20)
	var lost [][3]uint64
	var previous map[sim.StateID]uint64
	for generation := range 3 {
		first, live, closeLog := handedOver(t, dir)
		if previous != nil {
			for _, id := range testkit.ClusterEngines {
				if first.Ticks[id] != previous[id] {
					t.Fatalf("generation %d: %v was replayed to %d, the last one reached %d",
						generation, id, first.Ticks[id], previous[id])
				}
			}
		}
		lost = append(lost, heldAndNotTaken(t, dir, live)...)
		step(t, live, 25)
		closeLog()
		everyMessageIsTakenOnce(t, dir, lost)
		previous = map[sim.StateID]uint64{}
		for _, id := range testkit.ClusterEngines {
			previous[id] = first.Ticks[id] + 25
		}
	}
	recoverTheWorld(t, dir, everySeventhTick)
}

func TestAClusterThatRunsOnItsOwnAfterAHandoverWritesLogsThatRecover(t *testing.T) {
	dir := t.TempDir()
	aWorldThatWroteItsFilesAndStopped(t, dir, everySeventhTick, 0)
	crash(t, dir, testkit.Layer1, 12)
	first, live, closeLog := handedOver(t, dir)
	lost := heldAndNotTaken(t, dir, live)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	live.Run(ctx, time.Millisecond)
	closeLog()
	everyMessageIsTakenOnce(t, dir, lost)
	second := recoverTheWorld(t, dir, everySeventhTick)
	for _, id := range testkit.ClusterEngines {
		if second.Ticks[id] <= first.Ticks[id] {
			t.Fatalf("%v did not run: replayed to %d and then recovered to %d",
				id, first.Ticks[id], second.Ticks[id])
		}
	}
}

func TestACommandAfterAHandoverIsInTheLogAndRecovers(t *testing.T) {
	dir := t.TempDir()
	aWorldThatWroteItsFilesAndStopped(t, dir, everySeventhTick, 0)
	before := len(recordsOnDisk(t, dir, testkit.Layer1))
	_, live, closeLog := handedOver(t, dir)
	layer, ok := live.Get[*zonelayerstate.State](testkit.Layer1)
	if !ok {
		t.Fatal("the cluster has no first zonelayer")
	}
	if !layer.Send(command.ZoneLayerChatSendCommand{FromID: 1, Text: "after the handover"}) {
		t.Fatal("the queue is full")
	}
	step(t, live, 10)
	closeLog()
	commands := 0
	for _, r := range recordsOnDisk(t, dir, testkit.Layer1)[before:] {
		if r.Body.Kind() == eventlog.KindCommand {
			commands++
		}
	}
	if commands != 1 {
		t.Fatalf("%d commands in the log after the handover, want 1", commands)
	}
	recoverTheWorld(t, dir, everySeventhTick)
}

func TestANewDocumentOfTheConfigurationAfterAHandoverReachesTheEnginesAndRecovers(t *testing.T) {
	dir := t.TempDir()
	aWorldThatWroteItsFilesAndStopped(t, dir, everySeventhTick, 0)
	_, live, closeLog := handedOver(t, dir)
	if err := live.ConfigStore().Apply(map[string]string{
		"checkpoint.every_ticks": "7", "zonelayer.normal_attack.cooldown_ticks": "9",
	}); err != nil {
		t.Fatal(err)
	}
	step(t, live, 10)
	if got := live.ConfigEngine().State().Snapshot(); got == nil {
		t.Fatal("the configuration engine has no snapshot")
	}
	closeLog()
	recoverTheWorld(t, dir, everySeventhTick)
}

func TestAReplayThatDiffersFromItsLogIsNotHandedOver(t *testing.T) {
	dir := t.TempDir()
	aWorldThatWroteItsFilesAndStopped(t, dir, never, 0)
	// a world whose zonelayers heal at another period is not the one that was recorded
	other := testkit.WorldBuilder(t, 7, 3)
	result, err := replay.Recover(dir, other)
	if err == nil {
		t.Skip("the other world did the same as the log: nothing differs to refuse")
	}
	if result == nil || len(result.Divergences) == 0 {
		t.Fatalf("recover failed with no divergence: %v", err)
	}
	env, envErr := other.NewEnv()
	if envErr != nil {
		t.Fatal(envErr)
	}
	if _, err := result.Handover(other, env); err == nil {
		t.Fatal("a replay with a divergence was handed over")
	}
}

func TestAWorldThatRanLongAfterItsLastCheckpointIsHandedOver(t *testing.T) {
	dir := t.TempDir()
	aWorldThatWroteItsFilesAndStopped(t, dir, everySeventhTick, 400)
	crash(t, dir, testkit.Layer2, 250)
	first, live, closeLog := handedOver(t, dir)
	lost := heldAndNotTaken(t, dir, live)
	step(t, live, 100)
	closeLog()
	everyMessageIsTakenOnce(t, dir, lost)
	second := recoverTheWorld(t, dir, everySeventhTick)
	if second.Ticks[testkit.Layer2] != first.Ticks[testkit.Layer2]+100 {
		t.Fatalf("the cut zonelayer was recovered to %d", second.Ticks[testkit.Layer2])
	}
}
