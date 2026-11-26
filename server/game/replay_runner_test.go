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
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/server/game"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/command"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/game/testkit"
	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// runOn replays the logs from tick 0 to the recovery line of the logs.
func runOn(b replay.Builder, logs replay.Logs) (*replay.Result, error) {
	plan, err := replay.PlanFrom(logs, nil, nil)
	if err != nil {
		return nil, err
	}
	return replay.Run(b, plan)
}

// readAll reads the files of a log into memory, to count what they hold.
func readAll(t *testing.T, dir string, codecs *sim.Codecs) eventlog.MemoryLogs {
	t.Helper()
	engines, err := eventlog.Engines(dir)
	if err != nil {
		t.Fatal(err)
	}
	logs := eventlog.MemoryLogs{}
	for _, e := range engines {
		if logs[e], err = eventlog.ReadAll(dir, e, codecs); err != nil {
			t.Fatal(err)
		}
	}
	return logs
}

func countKind(logs eventlog.MemoryLogs, k eventlog.Kind) int {
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

func TestAReplayOfAWholeClusterRunMatchesItsLogFromMemoryAndFromFiles(t *testing.T) {
	dir := t.TempDir()
	c, mem, file := testkit.NewTeeWorld(t, dir)
	testkit.RunScenario(t, c, testkit.Sender{T: t})
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	fromFiles, err := eventlog.NewFileLogs(dir, c.Env.Codecs)
	if err != nil {
		t.Fatal(err)
	}
	counted := map[string]eventlog.MemoryLogs{
		"memory": mem.ByEngine(), "files": readAll(t, dir, c.Env.Codecs),
	}
	sources := map[string]replay.Logs{"memory": mem.ByEngine(), "files": fromFiles}
	for name, logs := range sources {
		for _, kind := range []eventlog.Kind{
			eventlog.KindInbound, eventlog.KindCommand, eventlog.KindHash,
			eventlog.KindStableRead,
		} {
			if countKind(counted[name], kind) == 0 {
				t.Fatalf("%s: the scenario recorded no record of kind %d", name, kind)
			}
		}
		result, err := runOn(testkit.WorldBuilder(t, 7, 10), logs)
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
	c, _, mem := testkit.NewWorld(t, 7, 10)
	testkit.RunScenario(t, c, testkit.Sender{T: t})
	layer := testkit.Layer1

	without := func(kind eventlog.Kind) replay.Logs {
		logs := mem.ByEngine()
		cut := slices.IndexFunc(logs[layer], func(r eventlog.Record) bool {
			c, isCommand := r.Body.(eventlog.Command)
			_, move := c.Command.(command.ZoneLayerCharacterMoveCommand)
			return r.Kind() == kind && (!isCommand || move)
		})
		if cut < 0 {
			t.Fatalf("no record of kind %d to take out", kind)
		}
		logs[layer] = slices.Delete(slices.Clone(logs[layer]), cut, cut+1)
		return logs
	}
	for name, logs := range map[string]replay.Logs{
		"a command taken out": without(eventlog.KindCommand),
		"a hash taken out":    without(eventlog.KindHash),
	} {
		result, err := runOn(testkit.WorldBuilder(t, 7, 10), logs)
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

	logs := mem.ByEngine()
	i := slices.IndexFunc(logs[layer], func(r eventlog.Record) bool {
		return r.Kind() == eventlog.KindHash
	})
	logs[layer] = slices.Clone(logs[layer])
	hash := logs[layer][i].Body.(eventlog.Hash)
	logs[layer][i].Body = eventlog.Hash{Value: hash.Value ^ 1} // not the hash of the state
	result, err := runOn(testkit.WorldBuilder(t, 7, 10), logs)
	if err != nil || len(result.Divergences) == 0 {
		t.Fatalf("a wrong hash was not found: %v, %v", result, err)
	}
}

func TestAReplayWithoutTheLogOfTheConfigurationEngineCutsTheEnginesThatReadTheConfiguration(
	t *testing.T,
) {
	c, _, mem := testkit.NewWorld(t, 7, 10)
	testkit.RunScenario(t, c, testkit.Sender{T: t})
	logs := mem.ByEngine()
	if len(logs[sim.ConfigStateID]) == 0 {
		t.Fatal("the scenario recorded nothing of the configuration engine")
	}
	delete(logs, sim.ConfigStateID)
	// an engine with no log has no tick that a recovery keeps, so every engine that read a copy of
	// the configuration is cut before the tick that read it, and nothing of what the
	// configuration changed is replayed against logs that do not hold it
	plan, err := replay.PlanFrom(logs, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cut := 0
	for engine, records := range logs {
		if plan.Limits()[engine] < records[len(records)-1].Tick {
			cut++
		}
	}
	if cut == 0 {
		t.Fatal("no engine was cut for want of the log of the configuration engine")
	}
	result, err := replay.Run(testkit.WorldBuilder(t, 7, 10), plan)
	if err != nil || len(result.Divergences) != 0 {
		t.Fatalf("the engines that are left do not replay: %v %v", err, result.Divergences)
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

	t.Run("refused as full", func(t *testing.T) {
		c, mem := everyTickHashed(t)
		testkit.WalkToTheEastExit(t, c, 1)
		// the queue of zonelayer 3 is full, and it does not run to empty it
		for seq := uint64(1); ; seq++ {
			chat := action.ZoneLayerChatSendAction{FromID: 1, Text: "fill"}
			fill := sim.NewActionMessage[*zonelayerstate.State](
				testkit.Layer3, onfail.Drop{}, chat,
			).Stamp(sim.Header{From: 99, Seq: seq})
			reason, ok := c.Env.Router.Deliver(fill)
			if ok {
				continue
			}
			if reason != sim.DeliveryFailureFull {
				t.Fatalf("the queue of zonelayer 3 refused with %v, want full", reason)
			}
			break
		}
		c.ZoneLayer(testkit.Layer1).Send(exitEast)
		for range 10 {
			c.ZoneLayer(testkit.Layer1).Step()
		}
		reasons := map[sim.DeliveryFailure]int{}
		for _, r := range mem.ByEngine()[testkit.Layer1] {
			if returned, isReturned := r.Body.(eventlog.Returned); isReturned {
				reasons[returned.Reason]++
			}
		}
		if reasons[sim.DeliveryFailureFull] != 1 || len(reasons) != 1 {
			t.Fatalf("zonelayer 1 had messages come back for %v, want one as full", reasons)
		}
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

// everyTickHashed is the world of testkit.NewWorld(1, 0) with the hash of every engine recorded
// at every tick.
func everyTickHashed(t *testing.T) (*game.Cluster, *eventlog.MemLog) {
	t.Helper()
	c, _, mem := testkit.NewWorld(t, 1, 0)
	// every engine records the hash of its state at every tick, so that a replay is compared with
	// the run at every tick and ends where the run ended
	every := map[string]string{"eventlog.hash_every_ticks": "1"}
	testkit.SetConfig(t, c, every)
	return c, mem
}

// replayAndCompare replays what the memory log holds of the world of testkit.NewWorld(1, 0)
// and requires that nothing differs and that zonelayer 1 ends as it did: the character put back
// where it was.
func replayAndCompare(t *testing.T, c *game.Cluster, mem *eventlog.MemLog) {
	t.Helper()
	logs := mem.ByEngine()
	if countKind(logs, eventlog.KindReturned) != 1 {
		t.Fatalf("%d records of a message that came back, want 1",
			countKind(logs, eventlog.KindReturned))
	}
	result, err := runOn(testkit.WorldBuilder(t, 1, 0), logs)
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
	c, _, mem := testkit.NewWorld(t, 7, 10)
	testkit.RunScenario(t, c, testkit.Sender{T: t})
	logs := mem.ByEngine()
	layer := testkit.Layer1
	at := slices.IndexFunc(logs[layer], func(r eventlog.Record) bool {
		inbound, isInbound := r.Body.(eventlog.Inbound)
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
		action.ZoneLayerChatSendAction{FromID: 1, Text: "not what was said"},
	).Stamp(logs[layer][at].Body.(eventlog.Inbound).Message.Head())
	logs[layer][at].Body = eventlog.Inbound{Message: forged}

	result, err := runOn(testkit.WorldBuilder(t, 7, 10), logs)
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
		c, _, _ := testkit.NewWorld(b, 7, 10)
		testkit.RunScenario(b, c, testkit.Sender{T: b})
	}
}

func BenchmarkReplayCluster(b *testing.B) {
	c, _, mem := testkit.NewWorld(b, 7, 10)
	testkit.RunScenario(b, c, testkit.Sender{T: b})
	logs := mem.ByEngine()
	b.ReportAllocs()
	var ticks uint64
	for b.Loop() {
		result, err := runOn(testkit.WorldBuilder(b, 7, 10), logs)
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
	replay.Logs
	opened, closed int
	opens          map[sim.StateID]int
	failOn         sim.StateID
	failOnOpen     int // fail the source that is opened for this time (1: indexing, 2: running)
	failAt         int // at this record
}

func (c *countingLogs) Open(e sim.StateID, from uint64) (eventlog.Source, error) {
	src, err := c.Logs.Open(e, from)
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
	eventlog.Source
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
	c, _, mem := testkit.NewWorld(t, 7, 10)
	testkit.RunScenario(t, c, testkit.Sender{T: t})
	logs := mem.ByEngine()

	counting := &countingLogs{Logs: logs}
	plan, err := replay.PlanFrom(counting, nil, nil) // reads each log once, for the line
	if err != nil {
		t.Fatal(err)
	}
	counting.opened, counting.closed = 0, 0
	result, err := replay.Run(testkit.WorldBuilder(t, 7, 10), plan)
	if err != nil || len(result.Divergences) != 0 {
		t.Fatalf("%v %v", err, result.Divergences)
	}
	if want := 2 * len(logs); counting.opened != want || counting.closed != want {
		t.Fatalf("%d logs opened and %d closed, want %d of each: one pass to index, one to run",
			counting.opened, counting.closed, want)
	}

	// the failure is met while the plan works out the line, while indexing, and while running
	for _, open := range []int{1, 2, 3} {
		failing := &countingLogs{
			Logs: logs, failOn: testkit.Layer1, failOnOpen: open, failAt: 5,
		}
		_, err = runOn(testkit.WorldBuilder(t, 7, 10), failing)
		if err == nil || !strings.Contains(err.Error(), "the disk went away") {
			t.Fatalf("a log that fails at the record 5 of its open number %d: %v", open, err)
		}
		if failing.closed != failing.opened {
			t.Fatalf("open %d: %d logs opened and %d closed", open, failing.opened, failing.closed)
		}
	}
}

func TestALogWhoseRecordsAreNotInTheOrderOfTheirTicksIsAnError(t *testing.T) {
	c, _, mem := testkit.NewWorld(t, 7, 10)
	testkit.RunScenario(t, c, testkit.Sender{T: t})
	logs := mem.ByEngine()
	layer := testkit.Layer1
	records := slices.Clone(logs[layer])
	last := len(records) - 1
	records[0], records[last] = records[last], records[0] // the last tick first
	logs[layer] = records
	builder := testkit.WorldBuilder(t, 7, 10)
	if _, err := runOn(builder, logs); err == nil {
		t.Fatal("a log out of the order of its ticks was replayed")
	}
}

// The files of a log say which seed the run had; a replay with another is not a replay of it.
func TestFilesMadeWithAnotherSeedThanTheBuilderHasAreRefused(t *testing.T) {
	dir := t.TempDir()
	c, _, file := testkit.NewTeeWorld(t, dir)
	testkit.RunScenario(t, c, testkit.Sender{T: t})
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	logs, err := eventlog.NewFileLogs(dir, c.Env.Codecs)
	if err != nil {
		t.Fatal(err)
	}
	_, err = runOn(testkit.WorldBuilder(t, 8, 10), logs)
	if err == nil || !strings.Contains(err.Error(), "seed") {
		t.Fatalf("files made with the seed 7 and a builder with 8: %v", err)
	}
	result, err := runOn(testkit.WorldBuilder(t, 7, 10), logs)
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
	c, _, file := testkit.NewTeeWorld(t, dir)
	testkit.RunScenario(t, c, testkit.Sender{T: t})
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	engines, err := eventlog.Engines(dir)
	if err != nil {
		t.Fatal(err)
	}
	planBefore, err := replay.MakePlan(dir, c.Env.Codecs)
	if err != nil {
		t.Fatal(err)
	}
	whole := planBefore.Limits()
	// a crash: the first zonelayer lost its log after tick 7, and the others kept theirs; what it
	// sent at ticks 10 and 30 was taken by the world and by the third zonelayer
	const kept = 7
	layerDir := filepath.Join(dir, "zonelayer-"+fmt.Sprint(testkit.Layer1.Local()))
	if err = segmentlog.CutAfter(layerDir, kept, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	plan, err := replay.MakePlan(dir, c.Env.Codecs)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Cut(); err != nil {
		t.Fatal(err)
	}
	limits := plan.Limits()
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
	logs, err := eventlog.NewFileLogs(dir, c.Env.Codecs)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runOn(testkit.WorldBuilder(t, 7, 10), logs)
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
