package game_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/checkpoint"
	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/replay"
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
	c.World.Send(command.WorldZoneLayerAddCommand{
		ZoneID: testkit.Zone1, Capacity: 2, RegenEvery: 10,
	})
	testkit.Steps(t, c, 20)
}

func TestAZonelayerThatTheWorldAsksForGetsTheNextIDAndTheWorldLearnsItFromTheAnswer(t *testing.T) {
	c, _, mem := testkit.NewWorld(t, 7, 10)
	testkit.Steps(t, c, 10)
	c.World.Send(command.WorldZoneLayerAddCommand{
		ZoneID: testkit.Zone1, Capacity: 2, RegenEvery: 10,
	})
	testkit.Steps(t, c, 1) // the world asks, and the cluster makes the engine and answers
	found, ok := c.Cluster.Find(layer4)
	if !ok {
		t.Fatalf("the engine of %v is not in the cluster", layer4)
	}
	made := found.(engineReading)
	if made.Tick() != 0 {
		t.Fatalf("the engine of %v is at the tick %d, want it made and not run",
			layer4, made.Tick())
	}
	if slices.Contains(c.World.State().ZoneLayerIDs(), layer4) {
		t.Fatal("the world lists the zonelayer before it took the answer that says its ID")
	}
	testkit.Steps(t, c, 1)
	if !slices.Contains(c.World.State().ZoneLayerIDs(), layer4) {
		t.Fatal("the world does not list the zonelayer after it took the answer")
	}
	testkit.Steps(t, c, 4)
	if made.Tick() != 5 {
		t.Fatalf("the new engine is at the tick %d after 5 steps, want 5", made.Tick())
	}
	first := mem.ByEngine()[layer4][0]
	started, isStarted := first.Body.(eventlog.Started)
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
		c.World.Send(command.WorldZoneLayerAddCommand{
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
		c.World.Send(testkit.RemoveLayerCommand{Layer: layer4})
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
	started := recordsOnDisk(t, dir, layer4)[0].Body.(eventlog.Started)
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
	world, _ := live.Get[*worldstate.State](testkit.World)
	if slices.Contains(world.State().ZoneLayerIDs(), layer4) {
		t.Fatal("the world lists a zonelayer that it asked for in a tick that it lost")
	}
	// its ID is not given again, though the log of the engine was cut to nothing
	if got := live.NewStateID(state.ZoneLayerType); got != sim.NewStateID(state.ZoneLayerType, 5) {
		t.Fatalf("the next zonelayer is %v, want the number 5", got)
	}
	closeLog()
}

func TestAZonelayerWhoseRemovalWasLostToACrashIsMadeAgainAndTheWorldThatForgotItIsLeftToTheGame(
	t *testing.T,
) {
	dir := t.TempDir()
	aWorldThatRan(t, dir, everySeventhTick, func(c *testkit.Cluster) {
		aLayerWasMade(c, t)
		c.World.Send(testkit.RemoveLayerCommand{Layer: layer4})
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
	world, _ := live.Get[*worldstate.State](testkit.World)
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

func TestTheWorldBeginsItsLogWithItsStartAskedForByNoEngineAndTheOthersWithOneAskedForByTheWorld(
	t *testing.T,
) {
	c, _, mem := testkit.NewWorld(t, 7, 10)
	testkit.Steps(t, c, 2)
	for _, id := range testkit.ClusterEngines {
		first := mem.ByEngine()[id][0]
		started, ok := first.Body.(eventlog.Started)
		if !ok || first.Tick != 1 || len(started.Snapshot) == 0 {
			t.Fatalf("the first record of %v is %+v, want its start", id, first)
		}
		_, depends := first.Dependency()
		if id == testkit.World && (started.Creator != 0 || depends) {
			t.Fatalf("the start of the world is %+v, which no engine asked for", started)
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
	c.World.Send(command.WorldZoneLayerAddCommand{ZoneID: testkit.Zone1, Capacity: 2})
	testkit.Steps(t, c, 3)
	if _, ok := c.Cluster.Find(layer4); ok {
		t.Fatal("an engine was made that the registry cannot make")
	}
	if slices.Contains(c.World.State().ZoneLayerIDs(), layer4) {
		t.Fatal("the world lists a zonelayer that was not made")
	}
	// the number 4 went to an engine that could not be made, and is not given again
	next := c.Cluster.NewStateID(state.ZoneLayerType)
	if next != sim.NewStateID(state.ZoneLayerType, 5) {
		t.Fatalf("the next zonelayer is %v, want the number 5", next)
	}
}

func TestARequestWhoseAnswerWasLostToACrashIsNotServedAgainAfterAHandoverAndItsAnswerComes(
	t *testing.T,
) {
	dir := t.TempDir()
	aWorldThatRan(t, dir, everySeventhTick, func(c *testkit.Cluster) { aLayerWasMade(c, t) })
	started := recordsOnDisk(t, dir, layer4)[0].Body.(eventlog.Started)
	// the world kept the tick at which it asked, and lost the tick that took the answer
	crash(t, dir, testkit.World, started.CreatorTick)
	_, live, closeLog := handedOver(t, dir)
	defer closeLog()
	world, _ := live.Get[*worldstate.State](testkit.World)
	if _, ok := live.Find(layer4); !ok {
		t.Fatal("the zonelayer that the world asked for is not in the cluster")
	}
	if slices.Contains(world.State().ZoneLayerIDs(), layer4) {
		t.Fatal("the world lists the zonelayer before it took the answer, which it lost")
	}
	engines := len(live.Engines())
	step(t, live, 5)
	if !slices.Contains(world.State().ZoneLayerIDs(), layer4) {
		t.Fatal("the answer did not come again: the world does not list the zonelayer")
	}
	if got := len(live.Engines()); got != engines {
		t.Fatalf("the cluster has %d engines, had %d: the request was served again", got, engines)
	}
	next := live.NewStateID(state.ZoneLayerType)
	if next != sim.NewStateID(state.ZoneLayerType, 5) {
		t.Fatalf("the next zonelayer is %v, want the number 5", next)
	}
}
