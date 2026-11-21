// Command world runs a small world through the whole flow of the core, one tick at a time, and
// prints what the clients would be told. There is no network and no database in the core: this is
// a script that sends commands to the engines and steps them in a fixed order.
//
// Run it with:
//
//	go run ./server/cmd/world
package main

import (
	"fmt"
	"log"

	"github.com/huykingsofm/project-b/server/internal/services/game/engine"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/command"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	worldevent "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world/event"
	zoneevent "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zone/event"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
	zonelayerevent "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer/event"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap"
)

// printer is the output of the engines: it shows each change with the scope that made it.
type printer struct{}

func show[C any](scope string, from action.Address, tick uint64, events []C) {
	for _, c := range events {
		fmt.Printf("    %-5s %d  t=%-3d %T %+v\n", scope, from.ID, tick, c, c)
	}
}

func (printer) EmitWorld(from action.Address, tick uint64, events []worldevent.Event) {
	show("world", from, tick, events)
}

func (printer) EmitZone(from action.Address, tick uint64, events []zoneevent.Event) {
	show("zone", from, tick, events)
}

func (printer) EmitZoneLayer(from action.Address, tick uint64, events []zonelayerevent.Event) {
	show("zonelayer", from, tick, events)
}

func main() {
	exitEast := tilemap.ExitDefinition{Min: tilemap.Tile{X: 9, Y: 0}, Max: tilemap.Tile{X: 9, Y: 4}, ToZoneID: 2, Arrival: tilemap.Tile{X: 1, Y: 2}}
	exitWest := tilemap.ExitDefinition{Min: tilemap.Tile{X: 0, Y: 0}, Max: tilemap.Tile{X: 0, Y: 4}, ToZoneID: 1, Arrival: tilemap.Tile{X: 8, Y: 2}}
	spawn := tilemap.Tile{X: 1, Y: 2}
	walls := []tilemap.Tile{{X: 5, Y: 0}, {X: 5, Y: 1}}

	cluster, err := engine.NewBuilder().
		WithSeed(1).
		WithWorldEmitter(printer{}).
		WithZoneEmitter(printer{}).
		WithZoneLayerEmitter(printer{}).
		WithWorld(engine.WorldParams{ID: 1}).
		WithZone(engine.ZoneParams{ID: 1, Tiles: tilemap.New(10, 5, spawn, walls, []tilemap.ExitDefinition{exitEast}), Adjacent: []state.ZoneID{2}}).
		WithZoneLayers(engine.ZoneLayerParams{ZoneID: 1, Count: 2, Capacity: 2, RegenEvery: 10}).
		WithZone(engine.ZoneParams{ID: 2, Tiles: tilemap.New(10, 5, spawn, nil, []tilemap.ExitDefinition{exitWest}), Adjacent: []state.ZoneID{1}}).
		WithZoneLayers(engine.ZoneLayerParams{ZoneID: 2, Count: 1, Capacity: 4, RegenEvery: 10}).
		Build()
	if err != nil {
		panic(err)
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
			log.Printf("%s does not enter: %v", c.name, err)
			continue
		}
		cluster.Zone(1).Send(command.EnterWorldZoneCommand{Character: character})
	}
	step("two characters enter zone 1: the zone places them in zonelayer 1, the zonelayer accepts them, the zone records them", 4)

	cluster.ZoneLayer(1).Send(command.ChatZoneLayerCommand{FromID: 1, Text: "hello zonelayer"})
	cluster.Zone(1).Send(command.ChatZoneCommand{FromID: 1, Text: "hello zone"})
	cluster.World.Send(command.ChatPrivateWorldCommand{FromID: 10, ToID: 20, Text: "psst"})
	step("chat: a zonelayer message stays in the zonelayer, a zone message goes to every zonelayer of the zone, a private message is the world's alone", 4)

	cluster.ZoneLayer(1).Send(command.GrantShieldZoneLayerCommand{CharacterID: 2, Duration: 100})
	step("a shield is granted to Binh: it stands in the plan from now on", 2)
	cluster.ZoneLayer(1).Send(command.NormalAttackZoneLayerCommand{CharacterID: 1, TargetID: 2})
	step("An attacks Binh: the attack adds a damage to the plan, the shield runs before it and blocks it", 3)
	cluster.ZoneLayer(1).Send(command.NormalAttackZoneLayerCommand{CharacterID: 1, TargetID: 2})
	step("a second attack after the cooldown: the shield is gone, health drops", 10)

	cluster.ZoneLayer(1).Send(command.MoveZoneLayerCommand{CharacterID: 1, Dir: state.East, Ticks: 3})
	cluster.ZoneLayer(1).Send(command.MoveZoneLayerCommand{CharacterID: 1, Dir: state.East, Ticks: 5})
	step("two runs of movement: the second waits its turn in the cooldown and starts when the first is done", 10)

	cluster.ZoneLayer(1).Send(command.UseExitZoneLayerCommand{CharacterID: 1, ExitEntityID: 1})
	step("An takes the east exit: the exit is optimistic, so An moves at once to a zonelayer of zone 2 that has room, without waiting for it to answer", 4)

	fmt.Printf("\ncharacters in zonelayer 1: %d, zonelayer 2: %d, zonelayer 3 (zone 2): %d\n",
		cluster.ZoneLayer(1).State().CharacterCount(), cluster.ZoneLayer(2).State().CharacterCount(), cluster.ZoneLayer(3).State().CharacterCount())
}
