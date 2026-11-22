// Command world runs a small world through the whole flow of the core, one tick at a time, and
// prints what the clients would be told. There is no network and no database in the core: this is
// a script that sends commands to the engines and steps them in a fixed order.
//
// Run it with:
//
//	go run ./server/cmd/world
//
// With -replay-dir it also writes the replay log of every engine to files under that directory, and
// reads them back at the end to say what each engine recorded:
//
//	go run ./server/cmd/world -replay-dir /tmp/replay
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game"
	"github.com/huykingsofm/project-b/server/game/sim/command"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/shared/logging"
)

// printer is the output of the engines: it shows each change with the engine that made it.
type printer struct{}

func (printer) Emit(from sim.StateID, tick uint64, events []sim.Event) {
	for _, c := range events {
		fmt.Printf("    %-5d t=%-3d %T %+v\n", from, tick, c, c)
	}
}

// The IDs of the states of the world: unique among all of them. The builder gives the zonelayers
// the IDs after the largest of these, in the order of their zones.
const (
	worldID  sim.StateID = 1
	zone1ID  sim.StateID = 2
	zone2ID  sim.StateID = 3
	layer1ID sim.StateID = 4
	layer2ID sim.StateID = 5
	layer3ID sim.StateID = 6
)

func main() {
	replayDir := flag.String(
		"replay-dir", "", "write the replay log of every engine under this directory",
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

	exitEast := tilemap.ExitDefinition{
		Min:      tilemap.Tile{X: 9, Y: 0},
		Max:      tilemap.Tile{X: 9, Y: 4},
		ToZoneID: zone2ID,
		Arrival:  tilemap.Tile{X: 1, Y: 2},
	}
	exitWest := tilemap.ExitDefinition{
		Min:      tilemap.Tile{X: 0, Y: 0},
		Max:      tilemap.Tile{X: 0, Y: 4},
		ToZoneID: zone1ID,
		Arrival:  tilemap.Tile{X: 8, Y: 2},
	}
	spawn := tilemap.Tile{X: 1, Y: 2}
	walls := []tilemap.Tile{{X: 5, Y: 0}, {X: 5, Y: 1}}

	zone1Tiles, err := tilemap.New(10, 5, spawn, walls, []tilemap.ExitDefinition{exitEast})
	if err != nil {
		fail("the map of zone 1 is not well formed", err)
	}
	zone2Tiles, err := tilemap.New(10, 5, spawn, nil, []tilemap.ExitDefinition{exitWest})
	if err != nil {
		fail("the map of zone 2 is not well formed", err)
	}

	seed := uint64(1)
	var replayLog replay.Log = replay.NopLog{}
	var replayFile *replay.FileLog
	if *replayDir != "" {
		replayFile = replay.NewFileLog(
			*replayDir,
			replay.FileLogOptions{
				Seed: seed, SegmentBytes: 1 << 20, SyncEvery: time.Second, Backlog: 64,
			},
		)
		replayLog = replayFile
	}

	cluster, err := game.NewBuilder().
		WithSeed(seed).
		WithReplayLog(replayLog).
		WithEmitter(printer{}).
		WithWorld(game.WorldParams{ID: worldID}).
		WithZone(game.ZoneParams{ID: zone1ID, Tiles: zone1Tiles, Adjacent: []sim.StateID{zone2ID}}).
		WithZoneLayers(
			game.ZoneLayerParams{ZoneID: zone1ID, Count: 2, Capacity: 2, RegenEvery: 10},
		).
		WithZone(game.ZoneParams{ID: zone2ID, Tiles: zone2Tiles, Adjacent: []sim.StateID{zone1ID}}).
		WithZoneLayers(
			game.ZoneLayerParams{ZoneID: zone2ID, Count: 1, Capacity: 4, RegenEvery: 10},
		).
		Build()
	if err != nil {
		fail("the cluster could not be built", err)
	}

	step := func(title string, n int) {
		fmt.Printf("\n== %s (%d ticks)\n", title, n)
		for range n {
			cluster.Step()
		}
	}

	for _, c := range []struct {
		id   state.CharacterID
		name string
	}{{1, "An"}, {2, "Binh"}} {
		character, err := zonelayerstate.NewCharacter(c.id, c.name, 100, 1000, 256)
		if err != nil {
			logging.Error("character does not enter", "name", c.name, "err", err)
			continue
		}
		cluster.Zone(zone1ID).Send(command.ZoneCharacterEnterCommand{Character: character})
	}
	step(
		"two characters enter zone 1: the zone places them in zonelayer 1, the zonelayer accepts "+
			"them, the zone records them",
		4,
	)

	cluster.ZoneLayer(layer1ID).Send(
		command.ZoneLayerChatSendCommand{FromID: 1, Text: "hello zonelayer"},
	)
	cluster.Zone(zone1ID).Send(command.ZoneChatSendCommand{FromID: 1, Text: "hello zone"})
	cluster.World.Send(command.WorldChatPrivateSendCommand{FromID: 10, ToID: 20, Text: "psst"})
	step(
		"chat: a zonelayer message stays in the zonelayer, a zone message goes to every zonelayer "+
			"of the zone, a private message is the world's alone",
		4,
	)

	cluster.ZoneLayer(layer1ID).Send(
		command.ZoneLayerCharacterShieldGrantCommand{CharacterID: 2, Duration: 100},
	)
	step("a shield is granted to Binh: it stands in the plan from now on", 2)
	cluster.ZoneLayer(layer1ID).Send(
		command.ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID: 1, TargetID: 2},
	)
	step(
		"An attacks Binh: the attack adds a damage to the plan, the shield runs before it and "+
			"blocks it",
		3,
	)
	cluster.ZoneLayer(layer1ID).Send(
		command.ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID: 1, TargetID: 2},
	)
	step("a second attack after the cooldown: the shield is gone, health drops", 10)

	cluster.ZoneLayer(layer1ID).Send(
		command.ZoneLayerCharacterMoveCommand{CharacterID: 1, Dir: state.East, Ticks: 3},
	)
	cluster.ZoneLayer(layer1ID).Send(
		command.ZoneLayerCharacterMoveCommand{CharacterID: 1, Dir: state.East, Ticks: 5},
	)
	step(
		"two runs of movement: the second waits its turn in the cooldown and starts when the "+
			"first is done",
		10,
	)

	cluster.ZoneLayer(layer1ID).Send(
		command.ZoneLayerCharacterExitToCommand{
			CharacterID: 1, ExitEntityID: state.NewEntityID(zone1ID, 1),
		},
	)
	step(
		"An takes the east exit: the exit is optimistic, so An moves at once to a zonelayer of "+
			"zone 2 that has room, without waiting for it to answer",
		4,
	)

	fmt.Printf("\ncharacters in zonelayer 1: %d, zonelayer 2: %d, zonelayer 3 (zone 2): %d\n",
		cluster.ZoneLayer(layer1ID).State().CharacterCount(),
		cluster.ZoneLayer(layer2ID).State().CharacterCount(),
		cluster.ZoneLayer(layer3ID).State().CharacterCount())

	if replayFile != nil {
		if err := replayFile.Close(); err != nil {
			fail("the replay log could not be written", err)
		}
		engines, err := replay.Engines(*replayDir)
		if err != nil {
			fail("the replay log could not be listed", err)
		}
		fmt.Printf("\nthe replay log under %s, read back:\n", *replayDir)
		for _, e := range engines {
			reader, err := replay.ReadFileLog(*replayDir, e, 0, cluster.Env.Codecs)
			if err != nil {
				fail("the replay log could not be read", err)
			}
			counts, last := map[replay.Kind]int{}, uint64(0)
			for reader.Next() {
				counts[reader.Record().Kind()]++
				last = reader.Record().Tick
			}
			readErr := reader.Err()
			reader.Close()
			if readErr != nil {
				fail("the replay log is damaged", readErr)
			}
			fmt.Printf("  engine %d: %d inbound, %d commands, %d config, %d hashes, %d stable "+
				"reads, up to tick %d\n",
				e, counts[replay.KindInbound], counts[replay.KindCommand],
				counts[replay.KindConfig], counts[replay.KindHash],
				counts[replay.KindStableRead], last)
		}
	}
}
