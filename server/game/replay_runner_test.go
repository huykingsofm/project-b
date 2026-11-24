package game_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/replay/runner"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/server/game"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/command"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/game/testkit"
	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// readAll reads the files of a log into memory, to count what they hold.
func readAll(t *testing.T, dir string, codecs *sim.Codecs) runner.MemoryLogs {
	t.Helper()
	engines, err := replay.Engines(dir)
	if err != nil {
		t.Fatal(err)
	}
	logs := runner.MemoryLogs{}
	for _, e := range engines {
		if logs[e], err = replay.ReadAll(dir, e, codecs); err != nil {
			t.Fatal(err)
		}
	}
	return logs
}

// logsOf groups the records of a memory log by engine.
func logsOf(mem *replay.MemLog) runner.MemoryLogs {
	logs := runner.MemoryLogs{}
	for _, r := range mem.Records {
		logs[r.Engine] = append(logs[r.Engine], r)
	}
	return logs
}

func countKind(logs runner.MemoryLogs, k replay.Kind) int {
	n := 0
	for _, records := range logs {
		for _, r := range records {
			if r.Kind() == k {
				n++
			}
		}
	}
	return n
}

// theConfigOfTheScenario is the one new version of the configuration that runScenario makes.
var theConfigOfTheScenario = runner.Options{
	ConfigVersions: map[uint64]map[string]string{
		1: {"zonelayer.normal_attack.cooldown_ticks": "5"},
	},
}

func TestAReplayOfAWholeClusterRunMatchesItsLogFromMemoryAndFromFiles(t *testing.T) {
	dir := t.TempDir()
	c, mem, file, cfg := testkit.NewTeeWorld(t, dir)
	testkit.RunScenario(t, c, cfg, testkit.Sender{T: t})
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	fromFiles, err := runner.FileLogs(dir, c.Env.Codecs)
	if err != nil {
		t.Fatal(err)
	}
	counted := map[string]runner.MemoryLogs{
		"memory": logsOf(mem), "files": readAll(t, dir, c.Env.Codecs),
	}
	sources := map[string]runner.Logs{"memory": logsOf(mem), "files": fromFiles}
	for name, logs := range sources {
		for _, kind := range []replay.Kind{
			replay.KindInbound, replay.KindCommand, replay.KindConfig, replay.KindHash,
			replay.KindStableRead,
		} {
			if countKind(counted[name], kind) == 0 {
				t.Fatalf("%s: the scenario recorded no record of kind %d", name, kind)
			}
		}
		result, err := runner.Run(testkit.WorldBuilder(t, 7, 10), logs, theConfigOfTheScenario)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(result.Divergences) != 0 {
			t.Fatalf("%s: the replay differs from the log:\n%s",
				name, strings.Join(result.Divergences, "\n"))
		}
		if _, ok := layerOf(result.Cluster, testkit.Layer3).State().CharacterByID(1); !ok {
			t.Fatalf("%s: in the replay the character that crossed to zone 2 is not there", name)
		}
		for _, e := range testkit.ClusterEngines {
			if len(counted[name][e]) > 0 && result.Ticks[e] == 0 {
				t.Fatalf("%s: %v was not replayed", name, e)
			}
		}
	}
}

func TestAReplayFindsALogThatWasChangedAndSaysWhichEngineDiffers(t *testing.T) {
	c, _, mem, cfg := testkit.NewWorld(t, 7, 10)
	testkit.RunScenario(t, c, cfg, testkit.Sender{T: t})
	layer := testkit.Layer1

	without := func(kind replay.Kind) runner.Logs {
		logs := logsOf(mem)
		cut := slices.IndexFunc(logs[layer], func(r replay.Record) bool {
			c, isCommand := r.Body.(replay.Command)
			_, move := c.Command.(command.ZoneLayerCharacterMoveCommand)
			return r.Kind() == kind && (!isCommand || move)
		})
		if cut < 0 {
			t.Fatalf("no record of kind %d to take out", kind)
		}
		logs[layer] = slices.Delete(slices.Clone(logs[layer]), cut, cut+1)
		return logs
	}
	for name, logs := range map[string]runner.Logs{
		"a command taken out": without(replay.KindCommand),
		"a hash taken out":    without(replay.KindHash),
	} {
		result, err := runner.Run(testkit.WorldBuilder(t, 7, 10), logs, theConfigOfTheScenario)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(result.Divergences) == 0 {
			t.Fatalf("%s: the replay found nothing wrong", name)
		}
		if !strings.Contains(result.Divergences[0], "zonelayer") {
			t.Fatalf("%s: the first difference is %q, want one of a zonelayer",
				name, result.Divergences[0])
		}
	}

	logs := logsOf(mem)
	i := slices.IndexFunc(logs[layer], func(r replay.Record) bool {
		return r.Kind() == replay.KindHash
	})
	logs[layer] = slices.Clone(logs[layer])
	hash := logs[layer][i].Body.(replay.Hash)
	logs[layer][i].Body = replay.Hash{Value: hash.Value ^ 1} // not the hash of the state
	result, err := runner.Run(testkit.WorldBuilder(t, 7, 10), logs, theConfigOfTheScenario)
	if err != nil || len(result.Divergences) == 0 {
		t.Fatalf("a wrong hash was not found: %v, %v", result, err)
	}
}

func TestAReplayWithoutTheConfigThatWasReadSaysSo(t *testing.T) {
	c, _, mem, cfg := testkit.NewWorld(t, 7, 10)
	testkit.RunScenario(t, c, cfg, testkit.Sender{T: t})
	result, err := runner.Run(testkit.WorldBuilder(t, 7, 10), logsOf(mem), runner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Divergences) == 0 || !strings.Contains(result.Divergences[0], "configuration") {
		t.Fatalf("the missing version of the configuration was not reported: %v",
			result.Divergences)
	}
}

// A message that the router refuses at once and one that comes back later are both inputs that the
// time decides, so the log holds them and the replay gives them back at the tick they were settled.
// exitEast asks character 1 to leave zone 1 by its exit on the east edge.
var exitEast = command.ZoneLayerCharacterExitToCommand{
	CharacterID: 1, ExitEntityID: testkit.EastExit,
}

func TestAReplayGivesBackTheMessagesThatCameBackAtTheTickTheyCameBack(t *testing.T) {
	t.Run("refused at once", func(t *testing.T) {
		c, mem := everyTickHashed(t)
		testkit.WalkToTheEastExit(t, c, 1)
		c.Env.Router.Retire(testkit.Layer3)
		c.ZoneLayer(testkit.Layer1).Send(exitEast)
		testkit.Steps(t, c, 10)
		replayAndCompare(t, c, mem)
	})

	t.Run("comes back later", func(t *testing.T) {
		c, mem := everyTickHashed(t)
		testkit.WalkToTheEastExit(t, c, 1)
		c.ZoneLayer(testkit.Layer1).Send(exitEast)
		// the message to zonelayer 3 is in its queue, which does not run
		c.ZoneLayer(testkit.Layer1).Step()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { c.ZoneLayer(testkit.Layer3).Run(ctx, make(chan time.Time)); close(done) }()
		cancel() // the zonelayer retires and gives back what waits in its queue
		<-done
		for range 3 {
			c.ZoneLayer(testkit.Layer1).Step()
		}
		replayAndCompare(t, c, mem)
	})
}

// hashEveryTick is the configuration that makes every engine record the hash of its state at every
// tick, so that a replay is compared with the run at every tick and ends where the run ended.
var hashEveryTick = runner.Options{
	ConfigVersions: map[uint64]map[string]string{1: {"replay_log.hash_every_ticks": "1"}},
}

// everyTickHashed is the world of testkit.NewWorld(1, 0) with the hash of every engine recorded
// at every tick.
func everyTickHashed(t *testing.T) (*game.Cluster, *replay.MemLog) {
	t.Helper()
	c, _, mem, cfg := testkit.NewWorld(t, 1, 0)
	if _, err := cfg.Apply(hashEveryTick.ConfigVersions[1]); err != nil {
		t.Fatal(err)
	}
	return c, mem
}

// replayAndCompare replays what the memory log holds of the world of testkit.NewWorld(1, 0)
// and requires that nothing differs and that zonelayer 1 ends as it did: the character put back
// where it was.
func replayAndCompare(t *testing.T, c *game.Cluster, mem *replay.MemLog) {
	t.Helper()
	logs := logsOf(mem)
	if countKind(logs, replay.KindReturned) != 1 {
		t.Fatalf("%d records of a message that came back, want 1",
			countKind(logs, replay.KindReturned))
	}
	result, err := runner.Run(testkit.WorldBuilder(t, 1, 0), logs, hashEveryTick)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Divergences) != 0 {
		t.Fatalf("the replay differs from the log:\n%s", strings.Join(result.Divergences, "\n"))
	}
	want, ok := c.ZoneLayer(testkit.Layer1).State().CharacterEntityByCharacterID(1)
	got, replayed := layerOf(result.Cluster, testkit.Layer1).State().CharacterEntityByCharacterID(1)
	if !ok || !replayed || got.Position() != want.Position() {
		t.Fatalf("the character is at %+v (%v) in the replay and at %+v (%v) in the run",
			got.Position(), replayed, want.Position(), ok)
	}
	gotHash := layerOf(result.Cluster, testkit.Layer1).State().Hash()
	wantHash := c.ZoneLayer(testkit.Layer1).State().Hash()
	if gotHash != wantHash {
		t.Fatalf("zonelayer 1 ends with the hash %d in the replay and %d in the run",
			gotHash, wantHash)
	}
}

func TestAReplayFindsASenderThatDoesNotMakeTheMessageTheReceiverRecorded(t *testing.T) {
	c, _, mem, cfg := testkit.NewWorld(t, 7, 10)
	testkit.RunScenario(t, c, cfg, testkit.Sender{T: t})
	logs := logsOf(mem)
	layer := testkit.Layer1
	at := slices.IndexFunc(logs[layer], func(r replay.Record) bool {
		inbound, isInbound := r.Body.(replay.Inbound)
		if !isInbound {
			return false
		}
		m, ok := inbound.Message.(*sim.ActionMessage[*zonelayerstate.State])
		return ok && m.Head().From == testkit.Zone1 &&
			len(m.Actions) > 0 && m.Actions[0].ID() == action.ZoneLayerChatSendActionID
	})
	if at < 0 {
		t.Fatal("zonelayer 1 took no chat line from zone 1")
	}
	logs[layer] = slices.Clone(logs[layer])
	forged := sim.NewActionMessage[*zonelayerstate.State](
		testkit.Layer1, onfail.Drop{},
		&action.ZoneLayerChatSendAction{FromID: 1, Text: "not what was said"},
	).Stamp(logs[layer][at].Body.(replay.Inbound).Message.Head())
	logs[layer][at].Body = replay.Inbound{Message: forged}

	result, err := runner.Run(testkit.WorldBuilder(t, 7, 10), logs, theConfigOfTheScenario)
	if err != nil {
		t.Fatal(err)
	}
	zone := fmt.Sprintf("zone %d:", testkit.Zone1)
	if len(result.Divergences) == 0 || !strings.HasPrefix(result.Divergences[0], zone) {
		t.Fatalf(
			"the zone sent what the log says the zonelayer did not take: %v", result.Divergences,
		)
	}
}

func BenchmarkRunScenarioLive(b *testing.B) {
	for b.Loop() {
		c, _, _, cfg := testkit.NewWorld(b, 7, 10)
		testkit.RunScenario(b, c, cfg, testkit.Sender{T: b})
	}
}

func BenchmarkReplayCluster(b *testing.B) {
	c, _, mem, cfg := testkit.NewWorld(b, 7, 10)
	testkit.RunScenario(b, c, cfg, testkit.Sender{T: b})
	logs := logsOf(mem)
	b.ReportAllocs()
	var ticks uint64
	for b.Loop() {
		result, err := runner.Run(testkit.WorldBuilder(b, 7, 10), logs, theConfigOfTheScenario)
		if err != nil || len(result.Divergences) != 0 {
			b.Fatalf("%v %v", err, result.Divergences)
		}
		ticks = 0
		for _, n := range result.Ticks {
			ticks += n
		}
	}
	b.ReportMetric(float64(ticks), "engine-ticks/op")
}

// countingLogs counts how many sources of the logs were opened and closed, and can make one of
// them fail.
type countingLogs struct {
	runner.Logs
	opened, closed int
	opens          map[sim.StateID]int
	failOn         sim.StateID
	failOnOpen     int // fail the source that is opened for this time (1: indexing, 2: running)
	failAt         int // at this record
}

func (c *countingLogs) Open(e sim.StateID) (runner.Source, error) {
	src, err := c.Logs.Open(e)
	if err != nil {
		return nil, err
	}
	c.opened++
	if c.opens == nil {
		c.opens = map[sim.StateID]int{}
	}
	c.opens[e]++
	fail := 0
	if e == c.failOn && c.opens[e] == c.failOnOpen {
		fail = c.failAt
	}
	return &countingSource{Source: src, logs: c, failAt: fail}, nil
}

type countingSource struct {
	runner.Source
	logs   *countingLogs
	failAt int
	n      int
}

func (s *countingSource) Next() bool {
	s.n++
	if s.failAt > 0 && s.n >= s.failAt {
		return false
	}
	return s.Source.Next()
}

func (s *countingSource) Err() error {
	if s.failAt > 0 && s.n >= s.failAt {
		return errors.New("the disk went away")
	}
	return s.Source.Err()
}

func (s *countingSource) Close() error { s.logs.closed++; return s.Source.Close() }

func TestEveryLogIsOpenedTwiceAndClosedAndOneThatFailsIsAnErrorNotAMatch(t *testing.T) {
	c, _, mem, cfg := testkit.NewWorld(t, 7, 10)
	testkit.RunScenario(t, c, cfg, testkit.Sender{T: t})
	logs := logsOf(mem)

	counting := &countingLogs{Logs: logs}
	result, err := runner.Run(testkit.WorldBuilder(t, 7, 10), counting, theConfigOfTheScenario)
	if err != nil || len(result.Divergences) != 0 {
		t.Fatalf("%v %v", err, result.Divergences)
	}
	if want := 2 * len(logs); counting.opened != want || counting.closed != want {
		t.Fatalf("%d logs opened and %d closed, want %d of each: one pass to index, one to run",
			counting.opened, counting.closed, want)
	}

	for _, open := range []int{1, 2} { // the failure is met while indexing, and while running
		failing := &countingLogs{
			Logs: logs, failOn: testkit.Layer1, failOnOpen: open, failAt: 5,
		}
		_, err = runner.Run(testkit.WorldBuilder(t, 7, 10), failing, theConfigOfTheScenario)
		if err == nil || !strings.Contains(err.Error(), "the disk went away") {
			t.Fatalf("a log that fails at the record 5 of its open number %d: %v", open, err)
		}
		if failing.closed != failing.opened {
			t.Fatalf("open %d: %d logs opened and %d closed", open, failing.opened, failing.closed)
		}
	}
}

func TestALogWhoseRecordsAreNotInTheOrderOfTheirTicksIsAnError(t *testing.T) {
	c, _, mem, cfg := testkit.NewWorld(t, 7, 10)
	testkit.RunScenario(t, c, cfg, testkit.Sender{T: t})
	logs := logsOf(mem)
	layer := testkit.Layer1
	records := slices.Clone(logs[layer])
	last := len(records) - 1
	records[0], records[last] = records[last], records[0] // the last tick first
	logs[layer] = records
	builder := testkit.WorldBuilder(t, 7, 10)
	if _, err := runner.Run(builder, logs, theConfigOfTheScenario); err == nil {
		t.Fatal("a log out of the order of its ticks was replayed")
	}
}

// The files of a log say which seed the run had; a replay with another is not a replay of it.
func TestFilesMadeWithAnotherSeedThanTheBuilderHasAreRefused(t *testing.T) {
	dir := t.TempDir()
	c, _, file, cfg := testkit.NewTeeWorld(t, dir)
	testkit.RunScenario(t, c, cfg, testkit.Sender{T: t})
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	logs, err := runner.FileLogs(dir, c.Env.Codecs)
	if err != nil {
		t.Fatal(err)
	}
	_, err = runner.Run(testkit.WorldBuilder(t, 8, 10), logs, theConfigOfTheScenario)
	if err == nil || !strings.Contains(err.Error(), "seed") {
		t.Fatalf("files made with the seed 7 and a builder with 8: %v", err)
	}
	result, err := runner.Run(testkit.WorldBuilder(t, 7, 10), logs, theConfigOfTheScenario)
	if err != nil || len(result.Divergences) != 0 {
		t.Fatalf("the same seed: %v %v", err, result.Divergences)
	}
}

// layerOf is the engine of a zonelayer of a cluster, or nil.
func layerOf(c *engine.Cluster, id sim.StateID) *engine.Engine[*zonelayerstate.State] {
	e, _ := c.Get[*zonelayerstate.State](id)
	return e
}

func TestAClusterWhoseLogsAreCutToTheRecoveryLineAfterALostTailStillReplaysWithNoDivergence(
	t *testing.T,
) {
	dir := t.TempDir()
	c, _, file, cfg := testkit.NewTeeWorld(t, dir)
	testkit.RunScenario(t, c, cfg, testkit.Sender{T: t})
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	engines, err := replay.Engines(dir)
	if err != nil {
		t.Fatal(err)
	}
	whole, err := replay.RecoveryLine(dir, c.Env.Codecs)
	if err != nil {
		t.Fatal(err)
	}
	// a crash: the first zonelayer lost its log after tick 7, and the others kept theirs; what it
	// sent at ticks 10 and 30 was taken by the world and by the third zonelayer
	const kept = 7
	layerDir := filepath.Join(dir, "zonelayer-"+fmt.Sprint(testkit.Layer1.Local()))
	if err = segmentlog.CutAfter(layerDir, kept); err != nil {
		t.Fatal(err)
	}
	limits, err := replay.CutToRecoveryLine(dir, c.Env.Codecs)
	if err != nil {
		t.Fatal(err)
	}
	if limits[testkit.Layer1] != kept {
		t.Fatalf("the zonelayer is cut at %d, want %d", limits[testkit.Layer1], kept)
	}
	cut := 0
	for _, e := range engines {
		if e != testkit.Layer1 && limits[e] < whole[e] {
			cut++
		}
	}
	if cut == 0 {
		t.Fatal("no engine took what the zonelayer sent after tick 7: the test checks nothing")
	}
	logs, err := runner.FileLogs(dir, c.Env.Codecs)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(testkit.WorldBuilder(t, 7, 10), logs, theConfigOfTheScenario)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Divergences) != 0 {
		t.Fatalf("the cut logs do not replay:\n%s", strings.Join(result.Divergences, "\n"))
	}
	for _, e := range engines {
		if result.Ticks[e] > limits[e] {
			t.Fatalf("%v was replayed to %d, past its limit %d", e, result.Ticks[e], limits[e])
		}
	}
}
