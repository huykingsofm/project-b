// Command world runs a small world through the whole flow of the core, one tick at a time, and
// prints what the clients would be told. There is no network and no database in the core: this is
// a script that sends commands to the engines and steps them in a fixed order.
//
// Run it with:
//
//	go run ./server/cmd/world
//
// With -new-dir it starts a new cluster that writes the event log and the checkpoints of every
// engine to files under that directory, and reads them back at the end to say what each engine
// recorded. It refuses a directory that already holds the logs of a cluster:
//
//	go run ./server/cmd/world -new-dir /tmp/replay
//
// With -replay-dir it starts from the replay of such a directory: it cuts the logs to what no crash
// takes back, makes each engine again from its latest checkpoint (or from the record that begins
// its log) and runs it on its log, compares what it does with what it recorded, and then goes on
// live, writing after the cut, for a few ticks:
//
//	go run ./server/cmd/world -replay-dir /tmp/replay
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/journal/checkpoint"
	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/journal/replay"
	"github.com/huykingsofm/project-b/platform/runtime/cluster"
	"github.com/huykingsofm/project-b/platform/runtime/cluster/stepping"
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game"
	"github.com/huykingsofm/project-b/server/game/sim/command"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/shared/logging"
)

// The IDs of the states of the world. The cluster gives the numbers 1, 2, 3 of each type in the
// order of the requests.
var (
	worldID  = sim.NewStateID(state.WorldType, 1)
	zone1ID  = sim.NewStateID(state.ZoneType, 1)
	zone2ID  = sim.NewStateID(state.ZoneType, 2)
	layer1ID = sim.NewStateID(state.ZoneLayerType, 1)
	layer2ID = sim.NewStateID(state.ZoneLayerType, 2)
	layer3ID = sim.NewStateID(state.ZoneLayerType, 3)
)

const (
	seed      = 1
	queueSize = 256
)

// printer is the output of the engines: it shows each change with the engine that made it.
type printer struct{}

func (printer) Emit(from sim.StateID, tick uint64, events []sim.Event) {
	for _, c := range events {
		fmt.Printf("    %-11s t=%-3d %T %+v\n", label(from), tick, c, c)
	}
}

// world is the cluster with the engines of the game that the script reaches for.
type world struct {
	*cluster.Cluster
	world *engine.Engine[*worldstate.State]
}

func (w world) zone(id sim.StateID) *engine.Engine[*zonestate.State] {
	e, _ := w.FindOfType[*zonestate.State](id)
	return e
}

func (w world) zoneLayer(id sim.StateID) *engine.Engine[*zonelayerstate.State] {
	e, _ := w.FindOfType[*zonelayerstate.State](id)
	return e
}

// step runs n ticks of every engine, as a titled part of the script.
func (w world) step(fail func(string, error), title string, n int) {
	fmt.Printf("\n== %s (%d ticks)\n", title, n)
	if err := stepping.Steps(w.Cluster, n); err != nil {
		fail("the cluster could not run a tick", err)
	}
}

// label says an engine by the type of its state and its number: "zonelayer 2".
func label(id sim.StateID) string {
	names := map[sim.StateType]string{
		state.WorldType: "world", state.ZoneType: "zone", state.ZoneLayerType: "zonelayer",
	}
	name, ok := names[id.Type()]
	if !ok {
		name = fmt.Sprintf("type %d", id.Type())
	}
	return fmt.Sprintf("%s %d", name, id.Local())
}

// printTicks prints the engines of the cluster whose state is a S, each with its tick.
func printTicks[S sim.State](cluster *cluster.Cluster) {
	for _, e := range cluster.AllOfType[S]() {
		fmt.Printf("  %-11s t=%d\n", label(e.StateID()), e.Tick())
	}
}

func main() {
	newDir := flag.String(
		"new-dir", "",
		"start a new cluster and write the event log and the checkpoints of every engine under "+
			"this directory, which must hold none",
	)
	replayDir := flag.String(
		"replay-dir", "",
		"start from the replay of the event log and the checkpoints under this directory, and go "+
			"on writing there",
	)
	flag.Parse()

	closeLogs, err := logging.Init(logging.NewWriterConsumer(os.Stderr),
		logging.WithCapacity(4096),
		logging.WithFlushEvery(time.Second))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer closeLogs()
	fail := func(msg string, err error) {
		logging.Error(msg, "err", err)
		closeLogs() // os.Exit skips the deferred call, and the line would be lost
		os.Exit(1)
	}
	if *newDir != "" && *replayDir != "" {
		fail("two ways to start", errors.New("give -new-dir or -replay-dir, not both"))
	}

	codecs, engines, err := game.NewCodecsAndEngines()
	if err != nil {
		fail("the codecs of the game cannot be made", err)
	}
	options := engine.Options{
		Seed: seed, QueueSize: queueSize, Codecs: codecs, Engines: engines, Emitter: printer{},
	}

	dir := *newDir + *replayDir
	var replayFile *eventlog.FileLog
	var checkpointFiles *checkpoint.FileWriter
	var result *replay.Result
	if *replayDir != "" {
		// the cut and the replay come first: no log may be open for writing
		result, err = replay.Recover(dir, options)
		if err != nil {
			fail("the cluster cannot be made again from its replay", err)
		}
	}
	if dir != "" {
		if *newDir != "" {
			if err := mustHoldNoCluster(dir); err != nil {
				fail("the directory cannot hold a new cluster", err)
			}
		}
		checkpointFiles = checkpoint.NewFileWriter(dir, 64)
		options.CheckpointWriter = checkpointFiles

		replayFile = eventlog.NewFileLog(dir, eventlog.FileLogOptions{
			Seed: seed, SegmentBytes: 1 << 20, SyncEvery: time.Second, Backlog: 64,
		})
		options.EventLog = replayFile
	}

	store := config.NewStore()
	env := engine.NewEnvironment(options)
	var w world
	if result != nil {
		cluster, err := result.Handover(env, store)
		if err != nil {
			fail("the cluster cannot go on from its replay", err)
		}
		w = world{Cluster: cluster}
		w.world, _ = cluster.FindOfType[*worldstate.State](worldID)
		fmt.Printf("\nthe cluster was made again from %s: %d engines, each at its tick:\n",
			dir, len(cluster.Engines()))
		printTicks[*worldstate.State](cluster)
		printTicks[*zonestate.State](cluster)
		printTicks[*zonelayerstate.State](cluster)
		step := func(title string, n int) { w.step(fail, title, n) }
		step("the cluster goes on from the logs", 5)
	} else {
		orchestrator, err := cluster.NewOrchestrator(dir, store)
		if err != nil {
			fail("the orchestrator could not be made", err)
		}
		cluster, err := cluster.NewCluster(env, orchestrator, nil)
		if err != nil {
			fail("the cluster could not be made", err)
		}
		w = world{Cluster: cluster}
		createWorld := sim.CreateEngineCommand{State: worldstate.NewState()}
		if !cluster.CoordinatorEngine().SendCommand(createWorld) {
			fail("the world could not be asked for",
				errors.New("the queue of the coordinator engine is full"))
		}
		for range 10 { // the coordinator asks, the cluster makes the world, the world runs
			if err := stepping.Step(cluster); err != nil {
				fail("the cluster could not run a tick", err)
			}
			if w.world, _ = cluster.FindOfType[*worldstate.State](worldID); w.world != nil {
				break
			}
		}
		if w.world == nil {
			fail("the world could not be made", errors.New("no world after 10 steps"))
		}
		runScript(w, fail)
	}

	if replayFile != nil {
		checkpointFiles.Close()
		if err := replayFile.Close(); err != nil {
			fail("the event log could not be written", err)
		}
		report(dir, w, fail)
	}
}

// mustHoldNoCluster is nil for a directory that holds no log of an engine: a new cluster would have
// the IDs of the ones that are there.
func mustHoldNoCluster(dir string) error {
	engines, err := eventlog.Engines(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(engines) > 0 {
		return fmt.Errorf("%s holds the logs of %d engines: use another directory, or -replay-dir",
			dir, len(engines))
	}
	return nil
}

// runScript makes the zones and the zonelayers of the world, and then puts the world through the
// flow of the core.
func runScript(w world, fail func(string, error)) {
	step := func(title string, n int) { w.step(fail, title, n) }
	if err := w.ConfigStore().Apply(
		map[string]string{"checkpoint.every_ticks": "10"},
	); err != nil {
		fail("the configuration is refused", err)
	}

	spawn := tilemap.Tile{X: 1, Y: 2}
	walls := []tilemap.Tile{{X: 5, Y: 0}, {X: 5, Y: 1}}

	zone1Tiles, err := tilemap.New(10, 5, spawn, walls)
	if err != nil {
		fail("the map of zone 1 is not well formed", err)
	}
	zone2Tiles, err := tilemap.New(10, 5, spawn, nil)
	if err != nil {
		fail("the map of zone 2 is not well formed", err)
	}
	for _, c := range []sim.Command[*worldstate.State]{
		command.WorldZoneAddCommand{Tiles: zone1Tiles},
		command.WorldZoneAddCommand{Tiles: zone2Tiles},
		command.WorldZoneLayerAddCommand{ZoneID: zone1ID, Capacity: 2, RegenEvery: 10},
		command.WorldZoneLayerAddCommand{ZoneID: zone1ID, Capacity: 2, RegenEvery: 10},
		command.WorldZoneLayerAddCommand{ZoneID: zone2ID, Capacity: 4, RegenEvery: 10},
	} {
		w.world.SendCommand(c)
	}
	step(
		"the world asks the cluster for two zones and three zonelayers, takes the answers, and "+
			"each zonelayer tells its zone that it is there",
		4,
	)

	// the zones are joined while the game runs: the east edge of zone 1 leads to the west of zone
	// 2, and the west edge of zone 2 back
	for _, c := range []sim.Command[*worldstate.State]{
		command.WorldZoneExitAddCommand{
			FromZoneID: zone1ID, ToZoneID: zone2ID,
			Area:    tilemap.AreaOf(tilemap.Tile{X: 9, Y: 0}, tilemap.Tile{X: 9, Y: 4}),
			Arrival: tilemap.Tile{X: 1, Y: 2}.Center(),
		},
		command.WorldZoneExitAddCommand{
			FromZoneID: zone2ID, ToZoneID: zone1ID,
			Area:    tilemap.AreaOf(tilemap.Tile{X: 0, Y: 0}, tilemap.Tile{X: 0, Y: 4}),
			Arrival: tilemap.Tile{X: 8, Y: 2}.Center(),
		},
	} {
		w.world.SendCommand(c)
	}
	step("the world joins the two zones with an exit on each side", 3)

	for _, c := range []struct {
		id   state.CharacterID
		name string
	}{{1, "An"}, {2, "Binh"}} {
		character, err := zonelayerstate.NewCharacter(c.id, c.name, 100, 1000, 256)
		if err != nil {
			logging.Error("character does not enter", "name", c.name, "err", err)
			continue
		}
		w.zone(zone1ID).SendCommand(command.ZoneCharacterEnterCommand{Character: character})
	}
	step(
		"two characters enter zone 1: the zone places them in zonelayer 1, the zonelayer accepts "+
			"them, the zone records them",
		4,
	)

	w.zoneLayer(layer1ID).SendCommand(
		command.ZoneLayerChatSendCommand{FromID: 1, Text: "hello zonelayer"},
	)
	w.zone(zone1ID).SendCommand(command.ZoneChatSendCommand{FromID: 1, Text: "hello zone"})
	w.world.SendCommand(command.WorldChatPrivateSendCommand{FromID: 10, ToID: 20, Text: "psst"})
	step(
		"chat: a zonelayer message stays in the zonelayer, a zone message goes to every zonelayer "+
			"of the zone, a private message is the world's alone",
		4,
	)

	w.zoneLayer(layer1ID).SendCommand(
		command.ZoneLayerCharacterShieldGrantCommand{CharacterID: 2, Duration: 100},
	)
	step("a shield is granted to Binh: it stands in the plan from now on", 2)
	w.zoneLayer(layer1ID).SendCommand(
		command.ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID: 1, TargetID: 2},
	)
	step(
		"An attacks Binh: the attack adds a damage to the plan, the shield runs before it and "+
			"blocks it",
		3,
	)
	w.zoneLayer(layer1ID).SendCommand(
		command.ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID: 1, TargetID: 2},
	)
	step("a second attack after the cooldown: the shield is gone, health drops", 10)

	w.zoneLayer(layer1ID).SendCommand(
		command.ZoneLayerCharacterMoveCommand{CharacterID: 1, Dir: state.East, Ticks: 3},
	)
	w.zoneLayer(layer1ID).SendCommand(
		command.ZoneLayerCharacterMoveCommand{CharacterID: 1, Dir: state.East, Ticks: 5},
	)
	step(
		"two runs of movement: the second waits its turn in the cooldown and starts when the "+
			"first is done",
		10,
	)

	w.zoneLayer(layer1ID).SendCommand(
		command.ZoneLayerCharacterExitToCommand{CharacterID: 1, ExitEntityID: 1},
	)
	step(
		"An takes the east exit: the exit is optimistic, so An moves at once to a zonelayer of "+
			"zone 2 that has room, without waiting for it to answer",
		4,
	)

	fmt.Printf("\ncharacters in zonelayer 1: %d, zonelayer 2: %d, zonelayer 3 (zone 2): %d\n",
		w.zoneLayer(layer1ID).State().CharacterCount(),
		w.zoneLayer(layer2ID).State().CharacterCount(),
		w.zoneLayer(layer3ID).State().CharacterCount())
}

// report reads the files under dir back and says what each engine recorded.
func report(dir string, w world, fail func(string, error)) {
	engines, err := eventlog.Engines(dir)
	if err != nil {
		fail("the event log could not be listed", err)
	}
	fmt.Printf("\nthe event log under %s, read back:\n", dir)
	for _, e := range engines {
		reader, err := eventlog.ReadFileLog(dir, e, 0, w.Env.Codecs)
		if err != nil {
			fail("the event log could not be read", err)
		}
		counts, last := map[eventlog.Kind]int{}, uint64(0)
		for reader.Next() {
			counts[reader.Record().Kind()]++
			last = reader.Record().Tick
		}
		readErr := reader.Err()
		reader.Close()
		if readErr != nil {
			fail("the event log is damaged", readErr)
		}
		ticks, err := checkpoint.Ticks(dir, e)
		if err != nil {
			fail("the checkpoints could not be listed", err)
		}
		fmt.Printf("  %s: %d inbound, %d commands, %d directives, %d hashes, %d stable reads, "+
			"up to tick %d; checkpoints at %v\n",
			label(e), counts[eventlog.KindInbound], counts[eventlog.KindCommand],
			counts[eventlog.KindDirective], counts[eventlog.KindHash],
			counts[eventlog.KindStableRead], last, ticks)
	}
}
