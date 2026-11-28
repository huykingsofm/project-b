package testkit

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/journal/eventlog"
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
	"github.com/huykingsofm/project-b/server/game/sim/tilemap/tilemaptest"
)

// The IDs of the states of the world that NewCluster makes. The cluster gives the numbers 1, 2, 3
// of each type in the order of the requests.
var (
	World  = sim.NewStateID(state.WorldType, 1)
	Zone1  = sim.NewStateID(state.ZoneType, 1)
	Zone2  = sim.NewStateID(state.ZoneType, 2)
	Layer1 = sim.NewStateID(state.ZoneLayerType, 1)
	Layer2 = sim.NewStateID(state.ZoneLayerType, 2)
	Layer3 = sim.NewStateID(state.ZoneLayerType, 3)
)

// EastExit is the ID of the exit on the east edge of Zone1, the first entity that the zone holds.
const EastExit state.EntityID = 1

// Options are what the cluster of the test world is made with, and what a replay of it needs: the
// seed, the queues of 256, and the codecs and the registry of the game with the fixtures of the
// tests. The event log, the emitter and the writer of checkpoints are the caller's to add.
func Options(t testing.TB, seed uint64) engine.Options {
	t.Helper()
	codecs, engines, err := game.NewCodecsAndEngines()
	if err != nil {
		t.Fatal(err)
	}
	if err := registerFixtures(codecs); err != nil {
		t.Fatal(err)
	}
	return engine.Options{Seed: seed, QueueSize: 256, Codecs: codecs, Engines: engines}
}

// Steps runs n ticks of every engine of the cluster, and fails the test when one fails.
func Steps(t testing.TB, c *Cluster, n int) {
	t.Helper()
	for range n {
		if err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
}

func Enter(
	t *testing.T,
	c *Cluster,
	zoneID sim.StateID,
	characterID state.CharacterID,
	name string,
) {
	t.Helper()
	EnterCharacter(t, c, zoneID, NewCharacter(characterID, name, 100, 1000, 0))
}

// EnterCharacter puts a character, as it is, in the world through a zone.
func EnterCharacter(
	t *testing.T, c *Cluster, zoneID sim.StateID, character zonelayerstate.Character,
) {
	t.Helper()
	if !c.Zone(zoneID).SendCommand(command.ZoneCharacterEnterCommand{Character: character}) {
		t.Fatal("zone queue full")
	}
	Steps(t, c, 4)
}

// WalkToTheEastExit enters a character in Zone1 and walks it to the exit.
func WalkToTheEastExit(t *testing.T, c *Cluster, characterID state.CharacterID) {
	t.Helper()
	Enter(t, c, Zone1, characterID, "An")
	c.ZoneLayer(Layer1).SendCommand(
		command.ZoneLayerCharacterMoveCommand{CharacterID: characterID, Dir: state.East, Ticks: 8},
	)
	Steps(t, c, 12)
}

// NewCharacter is a character of the values of a test, which are never invalid.
func NewCharacter(
	characterID state.CharacterID, name string, maxHealth, speed, radius int32,
) zonelayerstate.Character {
	c, err := zonelayerstate.NewCharacter(characterID, name, maxHealth, speed, radius)
	if err != nil {
		panic(err)
	}
	return c
}

// Cluster is the cluster of the test world: the engines of the platform and the ones of the game
// that a test reaches for by their IDs.
type Cluster struct {
	*cluster.Cluster
	World *engine.Engine[*worldstate.State]
}

// Make makes a world in the environment of the options, with the zones given, by the commands of
// the game: the zones are numbered 1, 2, ... in the order given, and the zonelayers 1, 2, ... in
// the order of their zones, and the exits are put in once every zone is there. The world is the
// engine World. It has run until the world, the zones and the zonelayers know each other, and the
// zones have their exits. The store is the configuration of the cluster; nil makes an
// empty one.
func Make(
	t testing.TB, o engine.Options, store *config.Store, zones ...ZoneSpec,
) *Cluster {
	t.Helper()
	if store == nil {
		store = config.NewStore()
	}
	orchestrator, err := cluster.NewOrchestrator(logDirectory(o), store)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := cluster.NewCluster(engine.NewEnvironment(o), orchestrator, nil)
	if err != nil {
		t.Fatal(err)
	}
	coordinator := cluster.CoordinatorEngine()
	if !coordinator.SendCommand(sim.CreateEngineCommand{State: worldstate.NewState()}) {
		t.Fatal("the queue of the coordinator engine is full")
	}
	// the coordinator engine asks, the cluster makes the world, the coordinator engine takes the
	// answer, and the world runs
	var world *engine.Engine[*worldstate.State]
	for range 10 {
		if err := stepping.Step(cluster); err != nil {
			t.Fatal(err)
		}
		if world, _ = cluster.FindOfType[*worldstate.State](World); world != nil {
			break
		}
	}
	if world == nil {
		t.Fatalf("the world %v was not made", World)
	}
	c := &Cluster{Cluster: cluster, World: world}
	var commands []sim.Command[*worldstate.State]
	for _, z := range zones {
		commands = append(commands, command.WorldZoneAddCommand{Tiles: z.Tiles})
	}
	for i, z := range zones {
		zoneID := sim.NewStateID(state.ZoneType, uint64(i+1))
		for _, l := range z.Layers {
			commands = append(commands, command.WorldZoneLayerAddCommand{
				ZoneID: zoneID, Capacity: l.Capacity, RegenEvery: l.RegenEvery,
			})
		}
	}
	for _, c := range commands {
		if !world.SendCommand(c) {
			t.Fatal("the queue of the world is full")
		}
	}
	// the world asks, the cluster makes the engines, the world takes the answers, the zonelayers
	// tell their zones
	Steps(t, c, 4)
	// the world joins the zones: it lists them now
	for i, z := range zones {
		for _, e := range z.Exits {
			from := sim.NewStateID(state.ZoneType, uint64(i+1))
			if !world.SendCommand(command.WorldZoneExitAddCommand{
				FromZoneID: from, Area: e.Area, ToZoneID: e.To, Arrival: e.Arrival,
			}) {
				t.Fatal("the queue of the world is full")
			}
		}
	}
	Steps(t, c, 3) // the world sends, the zone takes it
	return c
}

// NewCluster makes the test world in the environment of the options: Zone1 (Layer1 and Layer2, two
// characters each) and Zone2 (Layer3, one character), each map 10 by 5 tiles, with an exit on the
// east edge of Zone1 to Zone2 and one on the west edge of Zone2 back to Zone1. Every zonelayer
// heals every regenEvery ticks (0: never).
func NewCluster(t testing.TB, o engine.Options, regenEvery uint64) *Cluster {
	t.Helper()
	return NewClusterIn(t, o, nil, regenEvery)
}

// NewClusterIn is NewCluster with the configuration store of the cluster given.
func NewClusterIn(t testing.TB, o engine.Options, store *config.Store, regenEvery uint64) *Cluster {
	t.Helper()
	spawn := tilemap.Tile{X: 1, Y: 2}
	// the east edge of Zone1 leads to the west of Zone2, and the west edge of Zone2 back
	east := ExitSpec{
		Area: tilemap.AreaOf(tilemap.Tile{X: 9, Y: 0}, tilemap.Tile{X: 9, Y: 4}), To: Zone2,
		Arrival: tilemap.Tile{X: 1, Y: 2}.Center(),
	}
	west := ExitSpec{
		Area: tilemap.AreaOf(tilemap.Tile{X: 0, Y: 0}, tilemap.Tile{X: 0, Y: 4}), To: Zone1,
		Arrival: tilemap.Tile{X: 8, Y: 2}.Center(),
	}
	return Make(t, o, store,
		ZoneSpec{
			Tiles: tilemaptest.New(t, 10, 5, spawn, nil),
			Layers: []LayerSpec{
				{Capacity: 2, RegenEvery: regenEvery}, {Capacity: 2, RegenEvery: regenEvery},
			},
			Exits: []ExitSpec{east},
		},
		ZoneSpec{
			Tiles:  tilemaptest.New(t, 10, 5, spawn, nil),
			Layers: []LayerSpec{{Capacity: 1, RegenEvery: regenEvery}},
			Exits:  []ExitSpec{west},
		},
	)
}

// NewWorld makes the test world with a memory log and a Recorder of what the engines tell their
// clients.
func NewWorld(
	t testing.TB, seed uint64, regenEvery uint64,
) (*Cluster, *Recorder, *eventlog.MemLog) {
	t.Helper()
	rec, eventLog := &Recorder{}, &eventlog.MemLog{}
	o := Options(t, seed)
	o.EventLog, o.Emitter = eventLog, rec
	return NewCluster(t, o, regenEvery), rec, eventLog
}

// Zone returns the engine of a zone, or nil.
func (c *Cluster) Zone(zoneID sim.StateID) *engine.Engine[*zonestate.State] {
	e, _ := c.FindOfType[*zonestate.State](zoneID)
	return e
}

// ZoneLayer returns the engine of a zonelayer, or nil.
func (c *Cluster) ZoneLayer(zoneLayerID sim.StateID) *engine.Engine[*zonelayerstate.State] {
	e, _ := c.FindOfType[*zonelayerstate.State](zoneLayerID)
	return e
}

// Step runs one tick of every engine, one after the other (see stepping.Step).
func (c *Cluster) Step() error { return stepping.Step(c.Cluster) }

// LayerSpec says how many characters a zonelayer holds and how often it heals them.
type LayerSpec struct {
	Capacity   int32
	RegenEvery uint64
}

// ExitSpec says an exit of a zone: the area (in units) that leads to the zone To, where a character
// comes out at Arrival.
type ExitSpec struct {
	Area    state.Rect
	To      sim.StateID
	Arrival state.Vec
}

// ZoneSpec says the map of a zone, its zonelayers and its exits.
type ZoneSpec struct {
	Tiles  *tilemap.TileMap
	Layers []LayerSpec
	Exits  []ExitSpec
}

// logDirectory is the directory that the event log of the options writes to, and empty for a log
// that does not write files: the orchestrator of the cluster stores its file there.
func logDirectory(o engine.Options) string {
	if directory, ok := o.EventLog.(interface{ Dir() string }); ok {
		return directory.Dir()
	}
	return ""
}
