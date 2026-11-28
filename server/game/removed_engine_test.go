package game_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/journal/checkpoint"
	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/journal/replay"
	"github.com/huykingsofm/project-b/platform/runtime/cluster"
	"github.com/huykingsofm/project-b/platform/runtime/cluster/stepping"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/game/testkit"
)

// aWorldInWhichAnEngineWasRemoved writes the files of a world that ran for before ticks, removed
// the engine, and ran for after ticks more.
func aWorldInWhichAnEngineWasRemoved(
	t *testing.T, dir string, id sim.StateID, before, after int,
) {
	t.Helper()
	aWorldInWhichAnEngineWasRemovedBy(t, dir, id, before, after, func(c *cluster.Cluster) {
		if !stepping.RetireEngine(c, id) {
			t.Fatalf("the world has no engine %v", id)
		}
	})
}

// aWorldInWhichAnEngineWasRemovedBy is aWorldInWhichAnEngineWasRemoved with the removal made by
// remove, between the two runs of ticks.
func aWorldInWhichAnEngineWasRemovedBy(
	t *testing.T, dir string, id sim.StateID, before, after int, remove func(*cluster.Cluster),
) {
	t.Helper()
	store := config.NewStore()
	file := eventlog.NewFileLog(dir, eventlog.FileLogOptions{
		Seed: 7, SegmentBytes: 2048, SyncEvery: time.Hour, Backlog: 8,
		OnSafe: replay.NewLineCacheWriter(dir, store),
	})
	files := checkpoint.NewFileWriter(dir, 64)
	o := testkit.Options(t, 7)
	o.EventLog, o.CheckpointWriter = file, files
	c := testkit.NewClusterIn(t, o, store, 10)
	testkit.SetConfig(t, c, everySeventhTick)
	testkit.Steps(t, c, before)
	remove(c.Cluster)
	testkit.Steps(t, c, after)
	files.Close()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAReplayRemovesTheEngineThatItsLogSaysWasRemovedAndRunsNoTickOfIt(t *testing.T) {
	c, _, mem := testkit.NewWorld(t, 7, 10)
	testkit.Steps(t, c, 20)
	ranBefore := c.ZoneLayer(testkit.Layer3).Tick()
	stepping.RetireEngine(c.Cluster, testkit.Layer3)
	testkit.Steps(t, c, 20)
	result, err := runOn(testkit.Options(t, 7), mem.ByEngine())
	if err != nil || len(result.Divergences) != 0 {
		t.Fatalf("%v %v", err, result.Divergences)
	}
	if !slices.Equal(result.Retired, []sim.StateID{testkit.Layer3}) {
		t.Fatalf("the replay removed %v, want only the third zonelayer", result.Retired)
	}
	if _, ok := result.Cluster.Find(testkit.Layer3); ok {
		t.Fatal("the cluster of the replay still has the engine that was removed")
	}
	if got := result.Ticks[testkit.Layer3]; got != ranBefore+1 {
		t.Fatalf("the engine was replayed to the tick %d, want %d: its ticks and its removal",
			got, ranBefore+1)
	}
}

func TestALogWhoseRemovalIsNotAloneInItsTickOrIsFollowedByAnotherIsRefused(t *testing.T) {
	c, _, mem := testkit.NewWorld(t, 7, 10)
	testkit.Steps(t, c, 20)
	stepping.RetireEngine(c.Cluster, testkit.Layer3)
	testkit.Steps(t, c, 5)
	for name, forge := range map[string]func(logs eventlog.MemoryLogs){
		"another record in the tick of the removal": func(logs eventlog.MemoryLogs) {
			last := len(logs[testkit.Layer3]) - 1
			logs[testkit.Layer3] = slices.Insert(logs[testkit.Layer3], last,
				eventlog.Record{Engine: testkit.Layer3, Tick: 21, Body: eventlog.Hash{Value: 1}})
		},
		"a record after the removal": func(logs eventlog.MemoryLogs) {
			logs[testkit.Layer3] = append(logs[testkit.Layer3],
				eventlog.Record{Engine: testkit.Layer3, Tick: 22, Body: eventlog.Hash{Value: 1}})
		},
		"a read of a copy of the tick of the removal": func(logs eventlog.MemoryLogs) {
			logs[testkit.Zone2] = append(logs[testkit.Zone2], eventlog.Record{
				Engine: testkit.Zone2, Tick: 25,
				Body: eventlog.StableRead{Source: testkit.Layer3, CopyTick: 21},
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			logs := mem.ByEngine()
			for id := range logs {
				logs[id] = slices.Clone(logs[id])
			}
			forge(logs)
			_, err := runOn(testkit.Options(t, 7), logs)
			if err == nil {
				t.Fatal("a log that cannot be the log of an engine that was removed was replayed")
			}
			t.Log(err)
		})
	}
}

func TestAWorldInWhichAnEngineWasRemovedRecoversWithNoDivergence(t *testing.T) {
	dir := t.TempDir()
	aWorldInWhichAnEngineWasRemoved(t, dir, testkit.Layer3, 20, 20)
	result, err := replay.Recover(dir, testkit.Options(t, 7))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Retired, []sim.StateID{testkit.Layer3}) {
		t.Fatalf("the recovery removed %v, want only the third zonelayer", result.Retired)
	}
}

func TestAHandoverDoesNotMakeAgainAnEngineThatWasRemovedAndItsLogIsNotWrittenOn(t *testing.T) {
	dir := t.TempDir()
	aWorldInWhichAnEngineWasRemoved(t, dir, testkit.Layer3, 20, 20)
	before := len(recordsOnDisk(t, dir, testkit.Layer3))
	first, live, closeLog := handedOver(t, dir)
	if _, ok := live.Find(testkit.Layer3); ok {
		t.Fatal("the cluster that went live has the engine that was removed")
	}
	if len(live.Engines()) != len(first.Cluster.Engines()) {
		t.Fatalf("%d engines went live, %d were replayed", len(live.Engines()),
			len(first.Cluster.Engines()))
	}
	step(t, live, 30)
	closeLog()
	if got := len(recordsOnDisk(t, dir, testkit.Layer3)); got != before {
		t.Fatalf("the log of the removed engine has %d records, had %d", got, before)
	}
	second := recoverTheWorld(t, dir, everySeventhTick)
	if !slices.Equal(second.Retired, []sim.StateID{testkit.Layer3}) {
		t.Fatalf("the second recovery removed %v", second.Retired)
	}
	for _, id := range testkit.ClusterEngines {
		if id == testkit.Layer3 {
			continue
		}
		if want := first.Ticks[id] + 30; second.Ticks[id] != want {
			t.Fatalf("%v was recovered to the tick %d, want %d", id, second.Ticks[id], want)
		}
	}
}

func TestAMessageForAnEngineThatWasRemovedBeforeAHandoverComesBackAsRetired(t *testing.T) {
	dir := t.TempDir()
	aWorldInWhichAnEngineWasRemoved(t, dir, testkit.Layer3, 20, 20)
	_, live, closeLog := handedOver(t, dir)
	reason, ok := live.Env.Router.Deliver(sim.NewActionMessage[*zonelayerstate.State](
		testkit.Layer3, onfail.Drop{}, action.ZoneLayerChatSendAction{FromID: 1, Text: "x"},
	))
	if ok || reason != sim.DeliveryFailureRetired {
		t.Fatalf("a message for the removed engine: %v, %v, want it to come back as retired",
			reason, ok)
	}
	closeLog()
}

// layerTicksOfTheBuild is how many ticks a zonelayer of testkit.NewCluster has run when it returns.
const layerTicksOfTheBuild = 6

func TestAnEngineThatAnActionAskedToRemoveIsRemovedAtTheEndOfTheStepAndTheWorldRecovers(
	t *testing.T,
) {
	dir := t.TempDir()
	aWorldInWhichAnEngineWasRemovedBy(t, dir, testkit.Layer3, 20, 20,
		func(c *cluster.Cluster) {
			if !c.CoordinatorEngine().SendCommand(sim.RetireEngineCommand{Target: testkit.Layer3}) {
				t.Fatal("the queue of the coordinator engine is full")
			}
		})
	result, err := replay.Recover(dir, testkit.Options(t, 7))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Retired, []sim.StateID{testkit.Layer3}) {
		t.Fatalf("the recovery removed %v, want only the third zonelayer", result.Retired)
	}
	// the zonelayer ran the ticks of the build, of the 20 steps before the command, and of the two
	// steps in which the coordinator engine asked and the cluster ended it, and then its removal
	if got, want := result.Ticks[testkit.Layer3], uint64(layerTicksOfTheBuild+20+2+1); got != want {
		t.Fatalf("the engine was replayed to the tick %d, want %d", got, want)
	}
}

func TestAClusterThatRunsOnItsOwnAndRemovesAnEngineWritesLogsThatRecover(
	t *testing.T,
) {
	dir := t.TempDir()
	store := config.NewStore()
	file := eventlog.NewFileLog(dir, eventlog.FileLogOptions{
		Seed: 7, SegmentBytes: 2048, SyncEvery: time.Hour, Backlog: 8,
		OnSafe: replay.NewLineCacheWriter(dir, store),
	})
	files := checkpoint.NewFileWriter(dir, 64)
	o := testkit.Options(t, 7)
	o.EventLog, o.CheckpointWriter = file, files
	c := testkit.NewClusterIn(t, o, store, 10)
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() { c.Run(ctx, time.Millisecond); close(finished) }()
	time.Sleep(100 * time.Millisecond)
	c.Cluster.RequestEngineRetirement(testkit.Layer3)
	for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(time.Millisecond) {
		if _, ok := c.Cluster.Find(testkit.Layer3); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the engine was not removed in two seconds")
		}
	}
	time.Sleep(100 * time.Millisecond) // the others go on without it
	cancel()
	<-finished
	files.Close()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := replay.Recover(dir, testkit.Options(t, 7))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Retired, []sim.StateID{testkit.Layer3}) {
		t.Fatalf("the recovery removed %v, want only the third zonelayer", result.Retired)
	}
}

func TestAnEngineWhoseRemovalWasLostToACrashIsMadeAgainAndGoesOn(t *testing.T) {
	dir := t.TempDir()
	aWorldInWhichAnEngineWasRemoved(t, dir, testkit.Layer3, 20, 20)
	// the log of the engine lost its last frame, the one of its removal at the tick 21
	crash(t, dir, testkit.Layer3, 20)
	first, live, closeLog := handedOver(t, dir)
	if len(first.Retired) != 0 {
		t.Fatalf("the recovery removed %v: the log no longer says it was", first.Retired)
	}
	e, ok := live.Find(testkit.Layer3)
	if !ok || e.(engineReading).Tick() != 20 {
		t.Fatalf("the engine is not back at the tick 20: %v %v", e, ok)
	}
	step(t, live, 15)
	closeLog()
	second := recoverTheWorld(t, dir, everySeventhTick)
	if got := second.Ticks[testkit.Layer3]; got != 35 {
		t.Fatalf("the engine was recovered to the tick %d, want 35", got)
	}
}
