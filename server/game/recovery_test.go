package game_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/checkpoint"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game"
	"github.com/huykingsofm/project-b/server/game/testkit"
	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// aWorldThatWroteItsFilesAndStopped runs the scenario in a world that writes its event log and its
// checkpoints under dir at the period every says, goes on for idleTicks ticks more, and closes
// both.
func aWorldThatWroteItsFilesAndStopped(
	t testing.TB, dir string, every map[string]string, idleTicks int,
) *game.Cluster {
	t.Helper()
	cache := replay.NewLineCacheWriter(dir)
	file := eventlog.NewFileLog(dir, eventlog.FileLogOptions{
		Seed: 7, SegmentBytes: 2048, SyncEvery: time.Hour, Backlog: 8,
		OnSafe: cache,
	})
	files := checkpoint.NewFileWriter(dir, 64)
	c, err := testkit.WorldBuilder(t, 7, 10).
		WithEventLog(file).
		WithCheckpointWriter(files).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	cache.ReadConfigWith(c.Config)
	testkit.SetConfig(t, c, testkit.Sender{T: t}, every)
	testkit.RunScenarioKeeping(t, c, testkit.Sender{T: t}, every)
	testkit.Steps(t, c, idleTicks)
	files.Close()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return c
}

// recoverTheWorld recovers the world that aWorldThatWroteItsFilesAndStopped wrote, and fails the
// test when an engine differs from its log.
func recoverTheWorld(t testing.TB, dir string, every map[string]string) *replay.Result {
	t.Helper()
	result, err := replay.Recover(dir, testkit.WorldBuilder(t, 7, 10))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestARecoveryMakesEveryEngineFromItsCheckpointAndRunsItsLogWithNoDivergence(t *testing.T) {
	dir := t.TempDir()
	aWorldThatWroteItsFilesAndStopped(t, dir, everySeventhTick, 0)
	result := recoverTheWorld(t, dir, everySeventhTick)
	limits, starts := result.Plan.Limits(), result.Plan.Starts()
	for _, id := range testkit.ClusterEngines {
		start := starts[id]
		if start == 0 || start > limits[id] {
			t.Fatalf("%v starts at %d, line at %d: want a checkpoint", id, start, limits[id])
		}
		if got := result.Ticks[id]; got < start || got > limits[id] {
			t.Fatalf("%v ran to %d: want it between %d and %d", id, got, start, limits[id])
		}
	}
	if got := result.Ticks[testkit.Layer1]; got <= starts[testkit.Layer1] {
		t.Fatalf("the zonelayer ran no tick after its checkpoint (%d)", got)
	}
}

func TestARecoveryAfterAZonelayerLostItsTailGoesBackToAnEarlierCheckpointAndArchivesTheOthers(
	t *testing.T,
) {
	dir := t.TempDir()
	aWorldThatWroteItsFilesAndStopped(t, dir, everySeventhTick, 0)
	// a crash: the first zonelayer lost its log after tick 7, and the others kept theirs
	const kept = 7
	layerDir := eventlog.EngineDir(dir, testkit.Layer1)
	if err := segmentlog.CutAfter(layerDir, kept, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	before, err := checkpoint.Ticks(dir, testkit.Layer1)
	if err != nil {
		t.Fatal(err)
	}
	result := recoverTheWorld(t, dir, everySeventhTick)
	limit, start := result.Plan.Limits()[testkit.Layer1], result.Plan.Starts()[testkit.Layer1]
	if limit != kept || start > kept {
		t.Fatalf("the zonelayer is at %d and starts at %d: want a line at %d and a start at or "+
			"below it", limit, start, kept)
	}
	after, _ := checkpoint.Ticks(dir, testkit.Layer1)
	if len(after) >= len(before) {
		t.Fatalf("the zonelayer had the checkpoints %v and has %v: those above %d are to go",
			before, after, kept)
	}
	archived, _ := filepath.Glob(filepath.Join(
		dir, ".archive", "cut-1", filepath.Base(layerDir), "ckpt-*.snap",
	))
	if len(archived) != len(before)-len(after) {
		t.Fatalf("%d checkpoints in the archive, want %d", len(archived), len(before)-len(after))
	}
	for _, id := range testkit.ClusterEngines {
		if result.Ticks[id] > result.Plan.Limits()[id] {
			t.Fatalf("%v ran to %d, past its line %d",
				id, result.Ticks[id], result.Plan.Limits()[id])
		}
	}
}

func TestARecoveryWhoseCheckpointIsOfAnotherTickIsAnErrorAndNotADivergence(t *testing.T) {
	dir := t.TempDir()
	aWorldThatWroteItsFilesAndStopped(t, dir, everySeventhTick, 0)
	ticks, err := checkpoint.Ticks(dir, testkit.Layer1)
	if err != nil || len(ticks) < 2 {
		t.Fatalf("the checkpoints %v (%v)", ticks, err)
	}
	// the file of the last checkpoint holds the one before it
	older, err := checkpoint.Read(dir, testkit.Layer1, ticks[len(ticks)-2])
	if err != nil {
		t.Fatal(err)
	}
	last := checkpoint.Path(dir, testkit.Layer1, ticks[len(ticks)-1])
	if err := os.WriteFile(last, older, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := replay.Recover(dir, testkit.WorldBuilder(t, 7, 10)); err == nil {
		t.Fatal("a checkpoint of another tick was taken")
	}
}

// What a recovery costs of a world that ran the scenario and then 3,000 ticks more, and took a
// checkpoint every 500 ticks (see docs/benchmark/replay.md).
var everyFiveHundredTicks = map[string]string{"checkpoint.every_ticks": "500"}

const idleTicksOfTheBenchmarks = 3000

// aLongWorld is a directory of the files of a world that ran for 3,100 ticks.
func aLongWorld(b *testing.B) string {
	dir := b.TempDir()
	aWorldThatWroteItsFilesAndStopped(b, dir, everyFiveHundredTicks, idleTicksOfTheBenchmarks)
	return dir
}

func BenchmarkRecoveryLineWithTheCacheAtTheEndOfTheLogs(b *testing.B) {
	dir := aLongWorld(b)
	codecs := testkit.WorldBuilder(b, 7, 10).Codecs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := replay.MakePlan(dir, codecs); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRecoveryLineWithNoCache(b *testing.B) {
	dir := aLongWorld(b)
	if err := os.Remove(filepath.Join(dir, "recovery-line")); err != nil {
		b.Fatal(err)
	}
	codecs := testkit.WorldBuilder(b, 7, 10).Codecs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := replay.MakePlan(dir, codecs); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkReplay(b *testing.B, planOf func(dir string, c *sim.Codecs) (*replay.Plan, error)) {
	dir := aLongWorld(b)
	recoverTheWorld(b, dir, everyFiveHundredTicks)
	builder := testkit.WorldBuilder(b, 7, 10)
	plan, err := planOf(dir, builder.Codecs())
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		replayed, err := replay.Run(builder, plan)
		if err != nil || len(replayed.Divergences) != 0 {
			b.Fatalf("%v %v", err, replayed.Divergences)
		}
	}
}

func BenchmarkReplayFromTheCheckpoints(b *testing.B) {
	benchmarkReplay(b, replay.MakePlan)
}

func BenchmarkReplayFromTickZero(b *testing.B) {
	benchmarkReplay(b, func(dir string, c *sim.Codecs) (*replay.Plan, error) {
		logs, err := eventlog.NewFileLogs(dir, c)
		if err != nil {
			return nil, err
		}
		return replay.PlanFrom(logs, nil, nil)
	})
}

func BenchmarkRecover(b *testing.B) {
	dir := aLongWorld(b)
	b.ResetTimer()
	for b.Loop() {
		recoverTheWorld(b, dir, everyFiveHundredTicks)
	}
}
