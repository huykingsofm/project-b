package game_test

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/runtime/cluster"
	"github.com/huykingsofm/project-b/platform/runtime/cluster/stepping"
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/runtime/stable"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/server/game/sim/action"
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
	r, err := eventlog.ReadFileLog(dir, id, 0, testkit.Options(t, 7).Codecs)
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
func heldAndNotTaken(t *testing.T, dir string, live *cluster.Cluster) [][3]uint64 {
	t.Helper()
	taken := map[sim.StateID]map[[2]uint64]int{}
	var out [][3]uint64
	for _, handle := range live.Engines() {
		for _, m := range handle.(engineReading).GetSentMessages() {
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

func step(t *testing.T, c *cluster.Cluster, n int) {
	t.Helper()
	if err := stepping.Steps(c, n); err != nil {
		t.Fatal(err)
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
		if _, goesOn := live.Find(id); !goesOn {
			// an engine that a request made and that never ran is dropped when the engine that
			// asked lost the tick that took the response; it is made again with another ID
			continue
		}
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

func TestAHandoverOfAnEngineWhoseLogIsGoneDoesNotMakeItAgainAndCutsWhatDependsOnIt(t *testing.T) {
	dir := t.TempDir()
	aWorldThatWroteItsFilesAndStopped(t, dir, everySeventhTick, 0)
	if err := os.RemoveAll(eventlog.EngineDir(dir, testkit.Layer3)); err != nil {
		t.Fatal(err)
	}
	first, live, closeLog := handedOver(t, dir)
	if got := first.Ticks[testkit.Layer3]; got != 0 {
		t.Fatalf("the engine with no log was replayed to %d, want 0", got)
	}
	if _, ok := live.Find(testkit.Layer3); ok {
		t.Fatal("an engine that has no log was made: nothing says what it was")
	}
	published := live.Env.StableStore.(*stable.StateStore)
	for _, handle := range live.Engines() {
		e := handle.(engineReading)
		copy := published.Copy(0, 0, e.StateID())
		if e.Tick() > 0 && (copy == nil || copy.Tick() != e.Tick()) {
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
	layer, ok := live.FindOfType[*zonelayerstate.State](testkit.Layer1)
	if !ok {
		t.Fatal("the cluster has no first zonelayer")
	}
	if !layer.SendCommand(command.ZoneLayerChatSendCommand{FromID: 1, Text: "after the handover"}) {
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
	if got := live.CoordinatorEngine().State().Snapshot(); got == nil {
		t.Fatal("the coordinator engine has no snapshot")
	}
	closeLog()
	recoverTheWorld(t, dir, everySeventhTick)
}

func TestAReplayThatDiffersFromItsLogIsNotHandedOver(t *testing.T) {
	c, _, mem := testkit.NewWorld(t, 7, 10)
	testkit.Steps(t, c, 30)
	logs := mem.ByEngine()
	i := slices.IndexFunc(logs[testkit.Layer1], func(r eventlog.Record) bool {
		return r.Kind() == eventlog.KindHash
	})
	logs[testkit.Layer1] = slices.Clone(logs[testkit.Layer1])
	hash := logs[testkit.Layer1][i].Body.(eventlog.Hash)
	logs[testkit.Layer1][i].Body = eventlog.Hash{Value: hash.Value ^ 1} // not the hash of the state
	result, err := runOn(testkit.Options(t, 7), logs)
	if err != nil || len(result.Divergences) == 0 {
		t.Fatalf("a wrong hash was not found: %v, %v", result, err)
	}
	if _, err := result.Handover(
		engine.NewEnvironment(testkit.Options(t, 7)), config.NewStore(),
	); err == nil {
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

// removeLog takes the log of the engine out of the files.
func removeLog(dir string, id sim.StateID) error {
	return os.RemoveAll(eventlog.EngineDir(dir, id))
}

// testkitMessageFor is a message for the zonelayer with the ID.
func testkitMessageFor(id sim.StateID) sim.Message {
	return sim.NewActionMessage[*zonelayerstate.State](
		id, onfail.Drop{}, action.ZoneLayerChatSendAction{FromID: 1, Text: "x"},
	)
}
