package game_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/checkpoint"
	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/replay/runner"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game"
	"github.com/huykingsofm/project-b/server/game/sim/command"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/game/testkit"
)

var _ engine.CheckpointWriter = (*checkpoint.FileWriter)(nil)

// everySeventhTick is a configuration in which every engine takes a checkpoint every 7 ticks.
var everySeventhTick = map[string]string{"checkpoint.every_ticks": "7"}

// aWorldThatCheckpointsToFiles is the world of testkit.WorldBuilder, which takes a checkpoint every
// 7 ticks to files under dir and records its log in memory.
func aWorldThatCheckpointsToFiles(
	t *testing.T, dir string,
) (*game.Cluster, *replay.MemLog, *checkpoint.FileWriter) {
	t.Helper()
	cfg := config.NewStore()
	if _, err := cfg.Apply(everySeventhTick); err != nil {
		t.Fatal(err)
	}
	mem, files := &replay.MemLog{}, checkpoint.NewFileWriter(dir, 64)
	c, err := testkit.WorldBuilder(t, 7, 10).
		WithConfig(cfg).
		WithReplayLog(mem).
		WithCheckpointWriter(files).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	return c, mem, files
}

// hashOfRestored makes an engine from a checkpoint, in an environment of its own, and returns its
// tick and its hash.
func hashOfRestored(
	t *testing.T, c *game.Cluster, id sim.StateID, data []byte,
) (uint64, uint64) {
	t.Helper()
	env := aFreshEnv(c)
	var tick, hash uint64
	var err error
	switch id.Type() {
	case state.WorldType:
		var e *engine.Engine[*worldstate.State]
		if e, err = restoreEngine[*worldstate.State](data, env, 7); err == nil {
			tick, hash = e.Tick(), hashOf(e)
		}
	case state.ZoneType:
		var e *engine.Engine[*zonestate.State]
		if e, err = restoreEngine[*zonestate.State](data, env, 7); err == nil {
			tick, hash = e.Tick(), hashOf(e)
		}
	case state.ZoneLayerType:
		var e *engine.Engine[*zonelayerstate.State]
		if e, err = restoreEngine[*zonelayerstate.State](data, env, 7); err == nil {
			tick, hash = e.Tick(), hashOf(e)
		}
	default:
		t.Fatalf("%v is of no type of the game", id)
	}
	if err != nil {
		t.Fatalf("%v: %v", id, err)
	}
	return tick, hash
}

// loggedHashes returns the hash that the log has for each engine at each tick.
func loggedHashes(mem *replay.MemLog) map[sim.StateID]map[uint64]uint64 {
	out := map[sim.StateID]map[uint64]uint64{}
	for _, r := range mem.Records {
		if h, ok := r.Body.(replay.Hash); ok {
			if out[r.Engine] == nil {
				out[r.Engine] = map[uint64]uint64{}
			}
			out[r.Engine][r.Tick] = h.Value
		}
	}
	return out
}

// checkTheFilesOfEveryEngine checks that every engine of the cluster has checkpoints under dir,
// a period apart, and that each makes an engine with the hash that the log has at its tick.
func checkTheFilesOfEveryEngine(
	t *testing.T, c *game.Cluster, mem *replay.MemLog, dir string, every uint64,
) map[sim.StateID][]uint64 {
	t.Helper()
	hashes := loggedHashes(mem)
	out := map[sim.StateID][]uint64{}
	for _, id := range testkit.ClusterEngines {
		ticks, err := checkpoint.Ticks(dir, id)
		if err != nil {
			t.Fatal(err)
		}
		if len(ticks) < 2 {
			t.Fatalf("%v has the checkpoints %v: want some", id, ticks)
		}
		for i, tick := range ticks {
			if i > 0 && tick-ticks[i-1] != every {
				t.Fatalf("%v has checkpoints at %v: want them %d ticks apart", id, ticks, every)
			}
			data, err := checkpoint.Read(dir, id, tick)
			if err != nil {
				t.Fatal(err)
			}
			at, hash := hashOfRestored(t, c, id, data)
			logged, ok := hashes[id][tick]
			if at != tick || !ok || hash != logged {
				t.Fatalf("%v: the checkpoint of tick %d makes an engine at tick %d with the hash "+
					"%d; the log has %d (%v) at that tick", id, tick, at, hash, logged, ok)
			}
		}
		out[id] = ticks
	}
	return out
}

func TestEveryEngineWritesACheckpointFileEveryPeriodThatMakesItAgainWithTheHashOfTheLog(
	t *testing.T,
) {
	dir := t.TempDir()
	c, mem, files := aWorldThatCheckpointsToFiles(t, dir)
	testkit.Enter(t, c, testkit.Zone1, 2, "Binh")
	c.ZoneLayer(testkit.Layer1).Send(
		command.ZoneLayerCharacterShieldGrantCommand{CharacterID: 2, Duration: 100},
	)
	testkit.WalkToTheEastExit(t, c, 1) // crosses to zone 2: messages between the engines
	c.Zone(testkit.Zone1).Send(command.ZoneChatSendCommand{FromID: 2, Text: "hello"})
	testkit.Steps(t, c, 30)
	files.Close()
	if files.Dropped() != 0 || files.Failed() != 0 {
		t.Fatalf("dropped %d, failed %d", files.Dropped(), files.Failed())
	}

	firsts := map[uint64]bool{}
	for _, ticks := range checkTheFilesOfEveryEngine(t, c, mem, dir, 7) {
		firsts[ticks[0]] = true
	}
	if len(firsts) < 2 {
		t.Fatal("every engine takes its checkpoints at the same ticks")
	}

	// a replay, whose engines take no checkpoint, hashes at the same ticks
	result, err := runner.Run(testkit.WorldBuilder(t, 7, 10), logsOf(mem), runner.Options{
		ConfigVersions: map[uint64]map[string]string{1: everySeventhTick},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Divergences) != 0 {
		t.Fatalf("the replay differs from the log:\n%s", strings.Join(result.Divergences, "\n"))
	}
}

func TestEnginesThatRunOnTheirOwnGoroutinesWriteTheirCheckpointsWhileTheyGoOn(t *testing.T) {
	dir := t.TempDir()
	c, mem, files := aWorldThatCheckpointsToFiles(t, dir)
	testkit.Enter(t, c, testkit.Zone1, 1, "An")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Run(ctx, time.Millisecond)
	}()
	for _, id := range testkit.ClusterEngines { // until each has two at least
		for {
			ticks, err := checkpoint.Ticks(dir, id)
			if err != nil {
				t.Fatal(err)
			}
			if len(ticks) >= 2 {
				break
			}
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	<-done
	files.Close()
	checkTheFilesOfEveryEngine(t, c, mem, dir, 7)
}
