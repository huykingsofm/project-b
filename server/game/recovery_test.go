package game_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/checkpoint"
	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/recovery"
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/replay/runner"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game"
	"github.com/huykingsofm/project-b/server/game/testkit"
	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// configVersionsOf are the versions of the configuration of a world that takes a checkpoint at the
// period every says, all through the scenario.
func configVersionsOf(every map[string]string) map[uint64]map[string]string {
	return map[uint64]map[string]string{1: every, 2: testkit.ScenarioConfigVersion(every)}
}

// aWorldThatWroteItsFilesAndStopped runs the scenario in a world that writes its replay log and its
// checkpoints under dir at the period every says, goes on for idleTicks ticks more, and closes
// both.
func aWorldThatWroteItsFilesAndStopped(
	t testing.TB, dir string, every map[string]string, idleTicks int,
) *game.Cluster {
	t.Helper()
	cfg := config.NewStore()
	if _, err := cfg.Apply(every); err != nil {
		t.Fatal(err)
	}
	file := replay.NewFileLog(dir, replay.FileLogOptions{
		Seed: 7, SegmentBytes: 2048, SyncEvery: time.Hour, Backlog: 8,
		OnSafe: recovery.NewLineCacheWriter(dir, cfg),
	})
	files := checkpoint.NewFileWriter(dir, 64)
	c, err := testkit.WorldBuilder(t, 7, 10).
		WithConfig(cfg).
		WithReplayLog(file).
		WithCheckpointWriter(files).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	testkit.RunScenarioKeeping(t, c, cfg, testkit.Sender{T: t}, every)
	testkit.Steps(t, c, idleTicks)
	files.Close()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return c
}

// recoverTheWorld recovers the world that aWorldThatWroteItsFilesAndStopped wrote, and fails the
// test when an engine differs from its log.
func recoverTheWorld(t testing.TB, dir string, every map[string]string) *recovery.Result {
	t.Helper()
	result, err := recovery.Recover(dir, testkit.WorldBuilder(t, 7, 10),
		recovery.Options{ConfigVersions: configVersionsOf(every)})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Replay.Divergences) != 0 {
		t.Fatalf("the recovery differs from the logs:\n%s",
			strings.Join(result.Replay.Divergences, "\n"))
	}
	return result
}

func TestARecoveryMakesEveryEngineFromItsCheckpointAndRunsItsLogWithNoDivergence(t *testing.T) {
	dir := t.TempDir()
	aWorldThatWroteItsFilesAndStopped(t, dir, everySeventhTick, 0)
	result := recoverTheWorld(t, dir, everySeventhTick)
	for _, id := range testkit.ClusterEngines {
		start := result.Starts[id]
		if start == 0 || start > result.Limits[id] {
			t.Fatalf("%v starts at %d, with the line at %d: want a checkpoint",
				id, start, result.Limits[id])
		}
		if got := result.Replay.Ticks[id]; got < start || got > result.Limits[id] {
			t.Fatalf("%v ran to %d: want it between %d and %d", id, got, start, result.Limits[id])
		}
	}
	if got := result.Replay.Ticks[testkit.Layer1]; got <= result.Starts[testkit.Layer1] {
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
	layerDir := replay.EngineDir(dir, testkit.Layer1)
	if err := segmentlog.CutAfter(layerDir, kept, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	before, err := checkpoint.Ticks(dir, testkit.Layer1)
	if err != nil {
		t.Fatal(err)
	}
	result := recoverTheWorld(t, dir, everySeventhTick)
	if result.Limits[testkit.Layer1] != kept || result.Starts[testkit.Layer1] > kept {
		t.Fatalf("the zonelayer is at %d and starts at %d: want a line at %d and a start at or "+
			"below it", result.Limits[testkit.Layer1], result.Starts[testkit.Layer1], kept)
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
		if result.Replay.Ticks[id] > result.Limits[id] {
			t.Fatalf("%v ran to %d, past its line %d",
				id, result.Replay.Ticks[id], result.Limits[id])
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
	if _, err := recovery.Recover(
		dir, testkit.WorldBuilder(t, 7, 10),
		recovery.Options{ConfigVersions: configVersionsOf(everySeventhTick)},
	); err == nil {
		t.Fatal("a checkpoint of another tick was taken")
	}
}

// What a recovery costs of a world that ran the scenario and then 3,000 ticks more, and took a
// checkpoint every 500 ticks (see docs/benchmark/recovery.md).
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
		if _, err := recovery.RecoveryLine(dir, codecs); err != nil {
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
		if _, err := recovery.RecoveryLine(dir, codecs); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkReplay(b *testing.B, startsOf func(*recovery.Result) map[sim.StateID]uint64) {
	dir := aLongWorld(b)
	result := recoverTheWorld(b, dir, everyFiveHundredTicks)
	starts := startsOf(result)
	builder := testkit.WorldBuilder(b, 7, 10)
	b.ResetTimer()
	for b.Loop() {
		logs, err := runner.FileLogs(dir, builder.Codecs())
		if err != nil {
			b.Fatal(err)
		}
		replayed, err := runner.Run(builder, logs, runner.Options{
			ConfigVersions: configVersionsOf(everyFiveHundredTicks),
			Starts:         starts,
			Checkpoints:    recovery.CheckpointFiles(dir, starts),
		})
		if err != nil || len(replayed.Divergences) != 0 {
			b.Fatalf("%v %v", err, replayed.Divergences)
		}
	}
}

func BenchmarkReplayFromTheCheckpoints(b *testing.B) {
	benchmarkReplay(b, func(r *recovery.Result) map[sim.StateID]uint64 { return r.Starts })
}

func BenchmarkReplayFromTickZero(b *testing.B) {
	benchmarkReplay(b, func(*recovery.Result) map[sim.StateID]uint64 { return nil })
}

func BenchmarkRecover(b *testing.B) {
	dir := aLongWorld(b)
	b.ResetTimer()
	for b.Loop() {
		recoverTheWorld(b, dir, everyFiveHundredTicks)
	}
}
