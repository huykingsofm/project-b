package game_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/journal/checkpoint"
	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/journal/replay"
	"github.com/huykingsofm/project-b/platform/runtime/cluster/stepping"
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/command"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	"github.com/huykingsofm/project-b/server/game/testkit"
)

// layer4 is the first zonelayer that the world asks for after it was built: the build asked for the
// numbers 1 to 3.
var layer4 = sim.NewStateID(state.ZoneLayerType, 4)

// aWorldThatRan writes the files of a world that checkpoints every 7 ticks, and runs it with run,
// which makes and removes zonelayers by commands to the world, then closes the files.
func aWorldThatRan(
	t *testing.T, dir string, every map[string]string, run func(c *testkit.Cluster),
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
	testkit.SetConfig(t, c, every)
	run(c)
	files.Close()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

// aLayerWasMade makes a zonelayer by a command to the world after 10 ticks, and runs 20 more.
func aLayerWasMade(c *testkit.Cluster, t *testing.T) {
	testkit.Steps(t, c, 10)
	c.World.SendCommand(command.WorldZoneLayerAddCommand{
		ZoneID: testkit.Zone1, Capacity: 2, RegenEvery: 10,
	})
	testkit.Steps(t, c, 20)
}

func TestAZonelayerThatTheWorldAsksForGetsTheNextIDAndTheWorldLearnsItFromTheAnswer(t *testing.T) {
	c, _, mem := testkit.NewWorld(t, 7, 10)
	testkit.Steps(t, c, 10)
	c.World.SendCommand(command.WorldZoneLayerAddCommand{
		ZoneID: testkit.Zone1, Capacity: 2, RegenEvery: 10,
	})
	testkit.Steps(t, c, 1) // the world asks
	if _, ok := c.Cluster.Find(layer4); ok {
		t.Fatal("the engine is in the cluster before the cluster served the request")
	}
	// the cluster makes the engine and answers, and the world takes the answer
	testkit.Steps(t, c, 1)
	if _, ok := c.Cluster.Find(layer4); ok {
		t.Fatalf("the engine of %v is in the cluster before the world acknowledged", layer4)
	}
	if !slices.Contains(c.World.State().ZoneLayerIDs(), layer4) {
		t.Fatal("the world does not list the zonelayer after it took the answer")
	}
	testkit.Steps(t, c, 1) // the cluster takes the engine in, and it runs its tick 2
	found, ok := c.Cluster.Find(layer4)
	if !ok {
		t.Fatalf("the engine of %v is not in the cluster", layer4)
	}
	made := found.(engineReading)
	if made.Tick() != 2 {
		t.Fatalf("the engine of %v is at the tick %d, want 2", layer4, made.Tick())
	}
	testkit.Steps(t, c, 4)
	if made.Tick() != 6 {
		t.Fatalf("the new engine is at the tick %d after 7 steps, want 6", made.Tick())
	}
	first := mem.ByEngine()[layer4][0]
	started, isStarted := first.Body.(eventlog.Init)
	if !isStarted || first.Tick != 1 || started.Creator != testkit.World ||
		started.CreatorTick == 0 || started.DirectiveID == 0 || len(started.Snapshot) == 0 {
		t.Fatalf("the first record of its log is %+v, want its start by the world", first)
	}
}

func TestAWorldThatMadeAZonelayerRecoversItWithNoDivergenceFromItsCheckpointOrFromItsStart(
	t *testing.T,
) {
	for name, every := range map[string]map[string]string{
		"from a checkpoint":            everySeventhTick,
		"from the record of its start": everyFiveHundredTicks,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			aWorldThatRan(t, dir, every, func(c *testkit.Cluster) { aLayerWasMade(c, t) })
			result, err := replay.Recover(dir, testkit.Options(t, 7))
			if err != nil {
				t.Fatal(err)
			}
			found, ok := result.Cluster.Find(layer4)
			if !ok {
				t.Fatalf("the replay has no engine %v", layer4)
			}
			made := found.(engineReading)
			if got := result.Ticks[layer4]; got == 0 || made.Tick() != got {
				t.Fatalf("the engine was replayed to %d and is at %d", got, made.Tick())
			}
			if name == "from a checkpoint" && result.Plan.Starts()[layer4] == 0 {
				t.Fatal("the engine did not start from a checkpoint")
			}
		})
	}
}

func TestAHandoverMakesAgainTheZonelayerThatWasMadeWhileTheGameRanAndItGoesOn(t *testing.T) {
	dir := t.TempDir()
	aWorldThatRan(t, dir, everySeventhTick, func(c *testkit.Cluster) { aLayerWasMade(c, t) })
	first, live, closeLog := handedOver(t, dir)
	madeHandle, ok := live.Find(layer4)
	if !ok {
		t.Fatal("the cluster that went live has no engine of the zonelayer")
	}
	replayedHandle, _ := first.Cluster.Find(layer4)
	made, replayed := madeHandle.(engineReading), replayedHandle.(engineReading)
	if made.Tick() != replayed.Tick() ||
		made.Hash(sim.NewHasher()) != replayed.Hash(sim.NewHasher()) {
		t.Fatalf("the engine made again is at %d and the replayed one at %d, or the hash differs",
			made.Tick(), replayed.Tick())
	}
	step(t, live, 20)
	closeLog()
	second := recoverTheWorld(t, dir, everySeventhTick)
	if want := first.Ticks[layer4] + 20; second.Ticks[layer4] != want {
		t.Fatalf("the engine was recovered to %d, want %d", second.Ticks[layer4], want)
	}
}

func TestAClusterThatRunsOnItsOwnAndMakesAZonelayerWritesLogsThatRecover(t *testing.T) {
	dir := t.TempDir()
	aWorldThatRan(t, dir, everySeventhTick, func(c *testkit.Cluster) {
		ctx, cancel := context.WithCancel(context.Background())
		finished := make(chan struct{})
		go func() { c.Run(ctx, time.Millisecond); close(finished) }()
		time.Sleep(50 * time.Millisecond)
		c.World.SendCommand(command.WorldZoneLayerAddCommand{
			ZoneID: testkit.Zone1, Capacity: 2, RegenEvery: 10,
		})
		for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(time.Millisecond) {
			if _, ok := c.Cluster.Find(layer4); ok {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("the zonelayer was not made in two seconds")
			}
		}
		time.Sleep(100 * time.Millisecond)
		cancel()
		<-finished
	})
	result, err := replay.Recover(dir, testkit.Options(t, 7))
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Ticks[layer4]; got < 10 {
		t.Fatalf("the engine ran %d ticks on its goroutine, want more", got)
	}
}

func TestAZonelayerThatWasMadeAndThenRemovedIsOnlyRefusedMessagesAfterAHandover(t *testing.T) {
	dir := t.TempDir()
	aWorldThatRan(t, dir, everySeventhTick, func(c *testkit.Cluster) {
		aLayerWasMade(c, t)
		c.World.SendCommand(testkit.RemoveLayerCommand{Layer: layer4})
		testkit.Steps(t, c, 10)
	})
	first, live, closeLog := handedOver(t, dir)
	if !slices.Contains(first.Retired, layer4) {
		t.Fatalf("the replay removed %v, want the zonelayer", first.Retired)
	}
	if _, ok := live.Find(layer4); ok {
		t.Fatal("the cluster that went live has the zonelayer that was removed")
	}
	reason, ok := live.Env.Router.Deliver(testkitMessageFor(layer4))
	if ok || reason != sim.DeliveryFailureRetired {
		t.Fatalf("a message for it: %v %v, want it refused as retired", reason, ok)
	}
	step(t, live, 5)
	closeLog()
	second := recoverTheWorld(t, dir, everySeventhTick)
	if !slices.Contains(second.Retired, layer4) {
		t.Fatalf("the second recovery removed %v", second.Retired)
	}
}

func TestAZonelayerWhoseCreatorLostTheTickThatMadeItIsCutToNothingAndTheWorldForgetsIt(
	t *testing.T,
) {
	dir := t.TempDir()
	aWorldThatRan(t, dir, everySeventhTick, func(c *testkit.Cluster) { aLayerWasMade(c, t) })
	started := recordsOnDisk(t, dir, layer4)[0].Body.(eventlog.Init)
	// the world lost the tick at which it asked for the zonelayer and everything after it
	crash(t, dir, testkit.World, started.CreatorTick-1)
	first, live, closeLog := handedOver(t, dir)
	if first.Plan.Limits()[layer4] != 0 {
		t.Fatalf("the zonelayer has the limit %d, want it cut to nothing",
			first.Plan.Limits()[layer4])
	}
	if _, ok := live.Find(layer4); ok {
		t.Fatal("the cluster that went live has a zonelayer that the world never asked for")
	}
	world, _ := live.FindOfType[*worldstate.State](testkit.World)
	if slices.Contains(world.State().ZoneLayerIDs(), layer4) {
		t.Fatal("the world lists a zonelayer that it asked for in a tick that it lost")
	}
	// its ID is not given again, though the log of the engine was cut to nothing
	got, err := live.Orchestrator().NextStateID(state.ZoneLayerType)
	if err != nil || got != sim.NewStateID(state.ZoneLayerType, 5) {
		t.Fatalf("the next zonelayer is %v, %v, want the number 5", got, err)
	}
	closeLog()
}

func TestAZonelayerWhoseRemovalWasLostToACrashIsMadeAgainAndTheWorldThatForgotItIsLeftToTheGame(
	t *testing.T,
) {
	dir := t.TempDir()
	aWorldThatRan(t, dir, everySeventhTick, func(c *testkit.Cluster) {
		aLayerWasMade(c, t)
		c.World.SendCommand(testkit.RemoveLayerCommand{Layer: layer4})
		testkit.Steps(t, c, 10)
	})
	// the log of the zonelayer lost its last frame, the one of its removal
	records := recordsOnDisk(t, dir, layer4)
	crash(t, dir, layer4, records[len(records)-1].Tick-1)
	first, live, closeLog := handedOver(t, dir)
	if slices.Contains(first.Retired, layer4) {
		t.Fatal("the replay says the zonelayer was removed, but its log no longer says it")
	}
	if _, ok := live.Find(layer4); !ok {
		t.Fatal("the zonelayer was not made again from what its log keeps")
	}
	world, _ := live.FindOfType[*worldstate.State](testkit.World)
	if slices.Contains(world.State().ZoneLayerIDs(), layer4) {
		t.Fatal("the world lists the zonelayer that it forgot")
	}
	step(t, live, 5)
	closeLog()
	recoverTheWorld(t, dir, everySeventhTick)
}

func TestAHandoverAfterAnyEngineLostItsLogWhileAZonelayerWasMadeGoesOnAndRecovers(t *testing.T) {
	engines := append(slices.Clone(testkit.ClusterEngines), layer4)
	t.Run("cases", func(t *testing.T) { // the parallel cases end before this returns
		for _, id := range engines {
			for _, kept := range []uint64{1, 5, 11, 12, 15, 25} {
				t.Run(fmt.Sprintf("%v/kept %d", id, kept),
					func(t *testing.T) {
						t.Parallel()
						dir := t.TempDir()
						aWorldThatRan(t, dir, everySeventhTick,
							func(c *testkit.Cluster) { aLayerWasMade(c, t) })
						if last := recordsOnDisk(t, dir, id); len(last) == 0 ||
							last[len(last)-1].Tick <= kept {
							t.Skip("the engine has no tick above the one it keeps")
						}
						crash(t, dir, id, kept)
						first, live, closeLog := handedOver(t, dir)
						lost := heldAndNotTaken(t, dir, live)
						step(t, live, 20)
						closeLog()
						everyMessageIsTakenOnce(t, dir, lost)
						second := recoverTheWorld(t, dir, everySeventhTick)
						for engine, tick := range first.Ticks {
							if _, listed := live.Find(engine); !listed {
								continue
							}
							if second.Ticks[engine] != tick+20 {
								t.Fatalf("%v was recovered to %d, want %d",
									engine, second.Ticks[engine], tick+20)
							}
						}
					})
			}
		}
	})
}

func TestTheWorldBeginsItsLogWithAnInitAskedForByTheCoordinatorEngineAndTheOthersByTheWorld(
	t *testing.T,
) {
	c, _, mem := testkit.NewWorld(t, 7, 10)
	testkit.Steps(t, c, 2)
	for _, id := range testkit.ClusterEngines {
		first := mem.ByEngine()[id][0]
		started, ok := first.Body.(eventlog.Init)
		if !ok || first.Tick != 1 || len(started.Snapshot) == 0 {
			t.Fatalf("the first record of %v is %+v, want its start", id, first)
		}
		_, depends := first.Dependency()
		if id == testkit.World && (started.Creator != sim.CoordinatorStateID || !depends) {
			t.Fatalf("the start of the world is %+v, want it asked for by the coordinator engine",
				started)
		}
		if id != testkit.World && (started.Creator != testkit.World || !depends) {
			t.Fatalf("the start of %v is %+v, want it asked for by the world", id, started)
		}
	}
}

func TestAnEngineThatCannotBeMadeLeavesTheWorldWithoutItAndItsIDIsNotGivenAgain(t *testing.T) {
	c, _, _ := testkit.NewWorld(t, 7, 10)
	c.Env.EngineRegistry = engine.NewEngineRegistry() // knows no state of the game
	testkit.Steps(t, c, 10)
	c.World.SendCommand(command.WorldZoneLayerAddCommand{ZoneID: testkit.Zone1, Capacity: 2})
	testkit.Steps(t, c, 3)
	if _, ok := c.Cluster.Find(layer4); ok {
		t.Fatal("an engine was made that the registry cannot make")
	}
	if slices.Contains(c.World.State().ZoneLayerIDs(), layer4) {
		t.Fatal("the world lists a zonelayer that was not made")
	}
	// the number 4 went to an engine that could not be made, and is not given again
	next, err := c.Cluster.Orchestrator().NextStateID(state.ZoneLayerType)
	if err != nil || next != sim.NewStateID(state.ZoneLayerType, 5) {
		t.Fatalf("the next zonelayer is %v, %v, want the number 5", next, err)
	}
}

func TestAZonelayerWhoseResponseTheWorldLostToACrashIsMadeAgainWithAnotherIDAfterAHandover(
	t *testing.T,
) {
	dir := t.TempDir()
	aWorldThatRan(t, dir, everySeventhTick, func(c *testkit.Cluster) { aLayerWasMade(c, t) })
	init := recordsOnDisk(t, dir, layer4)[0].Body.(eventlog.Init)
	// the world kept the tick at which it asked, and lost the tick that took the response
	crash(t, dir, testkit.World, init.CreatorTick)
	_, live, closeLog := handedOver(t, dir)
	defer closeLog()
	world, _ := live.FindOfType[*worldstate.State](testkit.World)
	if _, ok := live.Find(layer4); ok {
		t.Fatal("the zonelayer that the world never took the response for is in the cluster")
	}
	layer5 := sim.NewStateID(state.ZoneLayerType, 5)
	step(t, live, 8)
	if slices.Contains(world.State().ZoneLayerIDs(), layer4) {
		t.Fatal("the world lists the zonelayer that was dropped")
	}
	if !slices.Contains(world.State().ZoneLayerIDs(), layer5) {
		t.Fatal("the request was not made again: the world lists no new zonelayer")
	}
	if _, ok := live.Find(layer5); !ok {
		t.Fatal("the engine of the request that was made again is not in the cluster")
	}
	records := recordsOnDisk(t, dir, layer4)
	if _, retired := records[len(records)-1].Body.(eventlog.Retired); !retired {
		t.Fatalf("the log of the dropped engine ends with %+v, want Retired",
			records[len(records)-1])
	}
	next, err := live.Orchestrator().NextStateID(state.ZoneLayerType)
	if err != nil || next != sim.NewStateID(state.ZoneLayerType, 6) {
		t.Fatalf("the next zonelayer is %v, %v, want the number 6", next, err)
	}
}

// responseTickFor is the tick at which the world took the response that made the engine, as its
// log under dir holds it.
func responseTickFor(t *testing.T, dir string, made sim.StateID) uint64 {
	t.Helper()
	for _, r := range recordsOnDisk(t, dir, testkit.World) {
		if d, ok := r.Body.(eventlog.Directive); ok && d.Response.Engine == made {
			return r.Tick
		}
	}
	t.Fatalf("the log of the world holds no response that made %v", made)
	return 0
}

// startedOf is the record Started in the log of the engine under dir.
func startedOf(t *testing.T, dir string, id sim.StateID) (eventlog.Record, eventlog.Started) {
	t.Helper()
	for _, r := range recordsOnDisk(t, dir, id) {
		if started, ok := r.Body.(eventlog.Started); ok {
			return r, started
		}
	}
	t.Fatalf("the log of %v holds no Started", id)
	return eventlog.Record{}, eventlog.Started{}
}

// aZonelayerThatLostAllButItsInitIsKeptAndStartedAgain checks a handover after the zonelayer lost
// its log after its tick 1, while the world kept the tick at which it took the response: the
// zonelayer is kept, writes Started again for that tick, goes on and recovers.
func aZonelayerThatLostAllButItsInitIsKeptAndStartedAgain(
	t *testing.T, dir string, wantWorld bool,
) {
	t.Helper()
	took := responseTickFor(t, dir, layer4)
	crash(t, dir, layer4, 1)
	first, live, closeLog := handedOver(t, dir)
	if first.Ticks[layer4] != 1 {
		t.Fatalf("the zonelayer was replayed to %d, want 1", first.Ticks[layer4])
	}
	if _, ok := live.Find(testkit.World); ok != wantWorld {
		t.Fatalf("the world is in the cluster that went live: %v, want %v", ok, wantWorld)
	}
	if _, ok := live.Find(layer4); !ok {
		t.Fatal("the zonelayer whose response the world took was dropped")
	}
	step(t, live, 5)
	closeLog()
	record, started := startedOf(t, dir, layer4)
	if record.Tick != 2 || started.Creator != testkit.World || started.ResponseTick != took {
		t.Fatalf("Started is %+v, want it at the tick 2 for the response that the world took at "+
			"its tick %d", record, took)
	}
	second := recoverTheWorld(t, dir, everySeventhTick)
	if second.Ticks[layer4] != 6 {
		t.Fatalf("the zonelayer was recovered to %d, want 6", second.Ticks[layer4])
	}
}

func TestAZonelayerThatLostAllButItsInitWhileTheWorldTookItsResponseIsKeptAndRunsOn(
	t *testing.T,
) {
	dir := t.TempDir()
	aWorldThatRan(t, dir, everySeventhTick, func(c *testkit.Cluster) { aLayerWasMade(c, t) })
	aZonelayerThatLostAllButItsInitIsKeptAndStartedAgain(t, dir, true)
}

func TestAZonelayerThatLostAllButItsInitIsKeptWhenTheWorldTookItsResponseAndThenRetired(
	t *testing.T,
) {
	dir := t.TempDir()
	aWorldThatRan(t, dir, everySeventhTick, func(c *testkit.Cluster) {
		aLayerWasMade(c, t)
		if !stepping.RetireEngine(c.Cluster, testkit.World) {
			t.Fatal("the world is not in the cluster")
		}
		testkit.Steps(t, c, 5)
	})
	aZonelayerThatLostAllButItsInitIsKeptAndStartedAgain(t, dir, false)
}
