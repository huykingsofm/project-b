package runner_test

import (
	"context"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/enginetest"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay/runner"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/onfail"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/server/internal/services/game/engine"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/command"
)

// logsOf groups the records of a memory log by engine.
func logsOf(mem *replay.MemLog) runner.Logs {
	logs := runner.Logs{}
	for _, r := range mem.Records {
		logs[r.Engine] = append(logs[r.Engine], r)
	}
	return logs
}

func countKind(logs runner.Logs, k replay.Kind) int {
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
	c, mem, file, cfg := enginetest.NewTeeWorld(t, dir)
	enginetest.RunScenario(t, c, cfg, enginetest.Sender{T: t})
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	fromFiles, err := runner.ReadLogs(dir)
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]runner.Logs{"memory": logsOf(mem), "files": fromFiles}
	for name, logs := range sources {
		for _, kind := range []replay.Kind{
			replay.KindInbound, replay.KindCommand, replay.KindConfig, replay.KindHash,
			replay.KindStableRead,
		} {
			if countKind(logs, kind) == 0 {
				t.Fatalf("%s: the scenario recorded no record of kind %d", name, kind)
			}
		}
		result, err := runner.Run(enginetest.WorldBuilder(t, 7, 10), logs, theConfigOfTheScenario)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(result.Divergences) != 0 {
			t.Fatalf("%s: the replay differs from the log:\n%s",
				name, strings.Join(result.Divergences, "\n"))
		}
		if _, ok := result.Cluster.ZoneLayer(3).State().CharacterByID(1); !ok {
			t.Fatalf("%s: in the replay the character that crossed to zone 2 is not there", name)
		}
		for _, e := range enginetest.ClusterEngines {
			if len(logs[e]) > 0 && result.Ticks[e] == 0 {
				t.Fatalf("%s: %v was not replayed", name, e)
			}
		}
	}
}

func TestAReplayFindsALogThatWasChangedAndSaysWhichEngineDiffers(t *testing.T) {
	c, _, mem, cfg := enginetest.NewWorld(t, 7, 10)
	enginetest.RunScenario(t, c, cfg, enginetest.Sender{T: t})
	layer := message.ZoneLayerAddress(1)

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
		result, err := runner.Run(enginetest.WorldBuilder(t, 7, 10), logs, theConfigOfTheScenario)
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
	result, err := runner.Run(enginetest.WorldBuilder(t, 7, 10), logs, theConfigOfTheScenario)
	if err != nil || len(result.Divergences) == 0 {
		t.Fatalf("a wrong hash was not found: %v, %v", result, err)
	}
}

func TestAReplayWithoutTheConfigThatWasReadSaysSo(t *testing.T) {
	c, _, mem, cfg := enginetest.NewWorld(t, 7, 10)
	enginetest.RunScenario(t, c, cfg, enginetest.Sender{T: t})
	result, err := runner.Run(enginetest.WorldBuilder(t, 7, 10), logsOf(mem), runner.Options{})
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
var exitEast = command.ZoneLayerCharacterExitToCommand{CharacterID: 1, ExitEntityID: 1}

func TestAReplayGivesBackTheMessagesThatCameBackAtTheTickTheyCameBack(t *testing.T) {
	t.Run("refused at once", func(t *testing.T) {
		c, mem := everyTickHashed(t)
		enginetest.WalkToTheEastExit(t, c, 1)
		c.Env.Router.Retire(message.ZoneLayerAddress(3))
		c.ZoneLayer(1).Send(exitEast)
		enginetest.Steps(c, 10)
		replayAndCompare(t, c, mem)
	})

	t.Run("comes back later", func(t *testing.T) {
		c, mem := everyTickHashed(t)
		enginetest.WalkToTheEastExit(t, c, 1)
		c.ZoneLayer(1).Send(exitEast)
		c.ZoneLayer(1).Step() // the message to zonelayer 3 is in its queue, which does not run
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { c.ZoneLayer(3).Run(ctx, make(chan time.Time)); close(done) }()
		cancel() // the zonelayer retires and gives back what waits in its queue
		<-done
		for range 3 {
			c.ZoneLayer(1).Step()
		}
		replayAndCompare(t, c, mem)
	})
}

// hashEveryTick is the configuration that makes every engine record the hash of its state at every
// tick, so that a replay is compared with the run at every tick and ends where the run ended.
var hashEveryTick = runner.Options{
	ConfigVersions: map[uint64]map[string]string{1: {"replay_log.hash_every_ticks": "1"}},
}

// everyTickHashed is the world of enginetest.NewWorld(1, 0) with the hash of every engine recorded
// at every tick.
func everyTickHashed(t *testing.T) (*engine.Cluster, *replay.MemLog) {
	t.Helper()
	c, _, mem, cfg := enginetest.NewWorld(t, 1, 0)
	if _, err := cfg.Apply(hashEveryTick.ConfigVersions[1]); err != nil {
		t.Fatal(err)
	}
	return c, mem
}

// replayAndCompare replays what the memory log holds of the world of enginetest.NewWorld(1, 0)
// and requires that nothing differs and that zonelayer 1 ends as it did: the character put back
// where it was.
func replayAndCompare(t *testing.T, c *engine.Cluster, mem *replay.MemLog) {
	t.Helper()
	logs := logsOf(mem)
	if countKind(logs, replay.KindReturned) != 1 {
		t.Fatalf("%d records of a message that came back, want 1",
			countKind(logs, replay.KindReturned))
	}
	result, err := runner.Run(enginetest.WorldBuilder(t, 1, 0), logs, hashEveryTick)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Divergences) != 0 {
		t.Fatalf("the replay differs from the log:\n%s", strings.Join(result.Divergences, "\n"))
	}
	want, ok := c.ZoneLayer(1).State().CharacterEntityByCharacterID(1)
	got, replayed := result.Cluster.ZoneLayer(1).State().CharacterEntityByCharacterID(1)
	if !ok || !replayed || got.Pos != want.Pos {
		t.Fatalf("the character is at %+v (%v) in the replay and at %+v (%v) in the run",
			got.Pos, replayed, want.Pos, ok)
	}
	gotHash, wantHash := result.Cluster.ZoneLayer(1).State().Hash(), c.ZoneLayer(1).State().Hash()
	if gotHash != wantHash {
		t.Fatalf("zonelayer 1 ends with the hash %d in the replay and %d in the run",
			gotHash, wantHash)
	}
}

func TestAReplayFindsASenderThatDoesNotMakeTheMessageTheReceiverRecorded(t *testing.T) {
	c, _, mem, cfg := enginetest.NewWorld(t, 7, 10)
	enginetest.RunScenario(t, c, cfg, enginetest.Sender{T: t})
	logs := logsOf(mem)
	layer := message.ZoneLayerAddress(1)
	at := slices.IndexFunc(logs[layer], func(r replay.Record) bool {
		inbound, isInbound := r.Body.(replay.Inbound)
		if !isInbound {
			return false
		}
		m, ok := inbound.Message.(*action.ScopedMessage[action.ZoneLayerAction])
		return ok && m.Head().From == message.ZoneAddress(1) &&
			len(m.Actions) > 0 && m.Actions[0].ID() == action.ZoneLayerChatSendActionID
	})
	if at < 0 {
		t.Fatal("zonelayer 1 took no chat line from zone 1")
	}
	logs[layer] = slices.Clone(logs[layer])
	forged := action.MessageToZoneLayer(
		1, onfail.Drop{},
		&action.ZoneLayerChatSendAction{FromID: 1, Text: "not what was said"},
	).Stamp(logs[layer][at].Body.(replay.Inbound).Message.Head())
	logs[layer][at].Body = replay.Inbound{Message: forged}

	result, err := runner.Run(enginetest.WorldBuilder(t, 7, 10), logs, theConfigOfTheScenario)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Divergences) == 0 || !strings.Contains(result.Divergences[0], "zone 1") {
		t.Fatalf("zone 1 sent what the log says zonelayer 1 did not take: %v", result.Divergences)
	}
}

func BenchmarkRunScenarioLive(b *testing.B) {
	for b.Loop() {
		c, _, _, cfg := enginetest.NewWorld(b, 7, 10)
		enginetest.RunScenario(b, c, cfg, enginetest.Sender{T: b})
	}
}

func BenchmarkReplayCluster(b *testing.B) {
	c, _, mem, cfg := enginetest.NewWorld(b, 7, 10)
	enginetest.RunScenario(b, c, cfg, enginetest.Sender{T: b})
	logs := logsOf(mem)
	b.ReportAllocs()
	var ticks uint64
	for b.Loop() {
		result, err := runner.Run(enginetest.WorldBuilder(b, 7, 10), logs, theConfigOfTheScenario)
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
