package engine_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/server/internal/config"
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

// recorder keeps what the engines give to the output.
type recorder struct {
	mu        sync.Mutex
	world     []worldevent.Event
	zone      []zoneevent.Event
	zonelayer []zonelayerevent.Event
}

func (r *recorder) EmitWorld(_ action.Address, _ uint64, c []worldevent.Event) {
	r.mu.Lock()
	r.world = append(r.world, c...)
	r.mu.Unlock()
}

func (r *recorder) EmitZone(_ action.Address, _ uint64, c []zoneevent.Event) {
	r.mu.Lock()
	r.zone = append(r.zone, c...)
	r.mu.Unlock()
}

func (r *recorder) EmitZoneLayer(_ action.Address, _ uint64, c []zonelayerevent.Event) {
	r.mu.Lock()
	r.zonelayer = append(r.zonelayer, c...)
	r.mu.Unlock()
}

// countChanges counts the events of type T.
func countChanges[T any, C any](r *recorder, events []C) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range events {
		if _, ok := any(c).(T); ok {
			n++
		}
	}
	return n
}

// CountWorld, CountZone and CountZoneLayer count the events of type T that the engines of a scope gave.
func (r *recorder) CountWorld[T any]() int     { return countChanges[T](r, r.world) }
func (r *recorder) CountZone[T any]() int      { return countChanges[T](r, r.zone) }
func (r *recorder) CountZoneLayer[T any]() int { return countChanges[T](r, r.zonelayer) }

// world: zone 1 (zonelayers 1 and 2, two characters each) and zone 2 (zonelayer 3, one character), each
// map 10 by 5 tiles, with an exit on the east edge of zone 1 to zone 2 and one on the west edge of zone
// 2 back to zone 1.
func newWorld(t *testing.T, seed uint64, regenEvery uint64) (*engine.Cluster, *recorder, *engine.MemReplayLog, *config.Store) {
	t.Helper()
	exit1 := tilemap.Exit{ID: 1, Min: state.Vec{X: 9, Y: 0}, Max: state.Vec{X: 9, Y: 4}, ToZoneID: 2, Arrival: state.Vec{X: 1, Y: 2}}
	exit2 := tilemap.Exit{ID: 1, Min: state.Vec{X: 0, Y: 0}, Max: state.Vec{X: 0, Y: 4}, ToZoneID: 1, Arrival: state.Vec{X: 8, Y: 2}}
	spawn := state.Vec{X: 1, Y: 2}
	rec, replayLog, cfg := &recorder{}, &engine.MemReplayLog{}, config.NewStore()
	c, err := engine.NewBuilder().
		WithConfig(cfg).
		WithSeed(seed).
		WithReplayLog(replayLog).
		WithWorldEmitter(rec).
		WithZoneEmitter(rec).
		WithZoneLayerEmitter(rec).
		WithWorld(engine.WorldParams{ID: 1}).
		WithZone(engine.ZoneParams{ID: 1, Tiles: tilemap.New(10, 5, spawn, nil, []tilemap.Exit{exit1}), Adjacent: []state.ZoneID{2}}).
		WithZoneLayers(engine.ZoneLayerParams{ZoneID: 1, Count: 2, Capacity: 2, RegenEvery: regenEvery}).
		WithZone(engine.ZoneParams{ID: 2, Tiles: tilemap.New(10, 5, spawn, nil, []tilemap.Exit{exit2}), Adjacent: []state.ZoneID{1}}).
		WithZoneLayers(engine.ZoneLayerParams{ZoneID: 2, Count: 1, Capacity: 1, RegenEvery: regenEvery}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	return c, rec, replayLog, cfg
}

func steps(c *engine.Cluster, n int) {
	for range n {
		c.Step()
	}
}

func enter(t *testing.T, c *engine.Cluster, zoneID state.ZoneID, charID state.CharacterID, name string) {
	t.Helper()
	enterCharacter(t, c, zoneID, zonelayerstate.NewCharacter(charID, name, 100))
}

// enterCharacter puts a character, as it is, in the world through a zone.
func enterCharacter(t *testing.T, c *engine.Cluster, zoneID state.ZoneID, character zonelayerstate.Character) {
	t.Helper()
	if !c.Zone(zoneID).Send(command.EnterWorldZoneCommand{Character: character}) {
		t.Fatal("zone queue full")
	}
	steps(c, 4)
}

// walkToTheEastExit enters a character in zone 1 and walks it to the exit.
func walkToTheEastExit(t *testing.T, c *engine.Cluster, charID state.CharacterID) {
	t.Helper()
	enter(t, c, 1, charID, "An")
	c.ZoneLayer(1).Send(command.MoveZoneLayerCommand{CharID: charID, Dir: state.East, Ticks: 8})
	steps(c, 12)
}

func TestEnteringTheWorldPutsTheCharacterInAZoneLayer(t *testing.T) {
	c, rec, _, _ := newWorld(t, 1, 0)
	enter(t, c, 1, 1, "An")

	if _, ok := c.ZoneLayer(1).State().EntityOfCharacter(1); !ok {
		t.Fatal("the character has no body in zonelayer 1")
	}
	if n := rec.CountZoneLayer[zonelayerevent.CharacterEntered](); n != 1 {
		t.Fatalf("the zonelayer told %d characters entered, want 1", n)
	}
}

func TestAFullZoneLayerSendsTheNextCharacterToAnotherZoneLayer(t *testing.T) {
	c, _, _, _ := newWorld(t, 1, 0)
	enter(t, c, 1, 1, "An")
	enter(t, c, 1, 2, "Binh")
	enter(t, c, 1, 3, "Chi") // zonelayer 1 holds two characters, so the third goes to zonelayer 2
	if _, ok := c.ZoneLayer(2).State().CharacterByID(3); !ok {
		t.Fatal("the third character is not in zonelayer 2")
	}
}

func TestRunsOfMovementAreAppliedInOrderThroughTheCooldown(t *testing.T) {
	c, _, _, _ := newWorld(t, 1, 0)
	enter(t, c, 1, 1, "An")
	l := c.ZoneLayer(1)
	start, _ := l.State().EntityOfCharacter(1)

	l.Send(command.MoveZoneLayerCommand{CharID: 1, Dir: state.East, Ticks: 3})
	l.Send(command.MoveZoneLayerCommand{CharID: 1, Dir: state.South, Ticks: 2}) // arrives while the first runs: it waits its turn
	steps(c, 8)

	end, _ := l.State().EntityOfCharacter(1)
	if want := (state.Vec{X: start.Pos.X + 3, Y: start.Pos.Y + 2}); end.Pos != want {
		t.Fatalf("position = %+v, want %+v", end.Pos, want)
	}
	if until := l.State().CooldownUntil[action.ZoneLayerCharacterMoveActionCooldownKey](1); until == 0 {
		t.Fatal("the movement left no cooldown")
	}
}

func TestChatInEachScope(t *testing.T) {
	c, rec, _, _ := newWorld(t, 1, 0)
	enter(t, c, 1, 1, "An")

	c.ZoneLayer(1).Send(command.ChatZoneLayerCommand{FromID: 1, Text: "zonelayer"})
	steps(c, 3)
	if n := rec.CountZoneLayer[zonelayerevent.ChatMessageSent](); n != 1 {
		t.Fatalf("a zonelayer message was heard by %d zonelayers, want 1", n)
	}
	if n := rec.CountZone[zoneevent.ChatMessageSent](); n != 0 {
		t.Fatalf("a zonelayer message reached the zone %d times", n)
	}

	c.Zone(1).Send(command.ChatZoneCommand{FromID: 1, Text: "zone"})
	steps(c, 3)
	if n := rec.CountZone[zoneevent.ChatMessageSent](); n != 1 {
		t.Fatalf("the zone recorded %d messages, want 1", n)
	}
	if n := rec.CountZoneLayer[zonelayerevent.ChatMessageSent](); n != 3 { // the first, and one for each of the two zonelayers of zone 1
		t.Fatalf("zonelayer messages heard = %d, want 3", n)
	}

	c.World.Send(command.ChatPrivateWorldCommand{FromID: 10, ToID: 20, Text: "private"})
	steps(c, 3)
	if n := rec.CountWorld[worldevent.PrivateChatMessageSent](); n != 1 {
		t.Fatalf("the world recorded %d private messages, want 1", n)
	}
	if n := rec.CountZoneLayer[zonelayerevent.ChatMessageSent](); n != 3 {
		t.Fatalf("a private message reached a zonelayer: %d zonelayer messages", n)
	}
}

func TestNormalAttackCooldownAndShield(t *testing.T) {
	c, _, _, _ := newWorld(t, 1, 0)
	enter(t, c, 1, 1, "An")
	enter(t, c, 1, 2, "Binh")
	l := c.ZoneLayer(1)
	health := func(charID state.CharacterID) int32 { ch, _ := l.State().CharacterByID(charID); return ch.Health }

	l.Send(command.NormalAttackZoneLayerCommand{CharID: 1, TargetID: 2})
	steps(c, 3)
	if health(2) != 99 {
		t.Fatalf("health after a hit = %d, want 99", health(2))
	}

	// The attack is on its cooldown: the second one does nothing.
	l.Send(command.NormalAttackZoneLayerCommand{CharID: 1, TargetID: 2})
	steps(c, 2)
	if health(2) != 99 {
		t.Fatalf("an attack in the cooldown did something: health %d", health(2))
	}

	// After the cooldown the attack lands again.
	steps(c, 6)
	l.Send(command.NormalAttackZoneLayerCommand{CharID: 1, TargetID: 2})
	steps(c, 3)
	if health(2) != 98 {
		t.Fatalf("the attack after the cooldown did not land: health %d, want 98", health(2))
	}

	// A shield stands in the plan, runs before the damage lands, and blocks the first one only.
	l.Send(command.GrantShieldZoneLayerCommand{CharID: 2, Duration: 1000})
	steps(c, 8)
	l.Send(command.NormalAttackZoneLayerCommand{CharID: 1, TargetID: 2})
	steps(c, 3)
	if health(2) != 98 {
		t.Fatalf("a shield did not block a hit: health %d", health(2))
	}
	steps(c, 8)
	l.Send(command.NormalAttackZoneLayerCommand{CharID: 1, TargetID: 2})
	steps(c, 3)
	if health(2) != 97 {
		t.Fatalf("the shield blocked a second hit: health %d, want 97", health(2))
	}
}

func TestAttackOnATargetThatIsNotThereDoesNothing(t *testing.T) {
	c, _, _, _ := newWorld(t, 1, 0)
	enter(t, c, 1, 1, "An")
	l := c.ZoneLayer(1)
	l.Send(command.NormalAttackZoneLayerCommand{CharID: 1, TargetID: 9})
	steps(c, 3)
	if ch, _ := l.State().CharacterByID(1); ch.Health != 100 {
		t.Fatalf("an attack on nobody changed health to %d", ch.Health)
	}
}

func TestAKilledCharacterStaysDead(t *testing.T) {
	c, _, _, _ := newWorld(t, 1, 0)
	enter(t, c, 1, 1, "An")
	enterCharacter(t, c, 1, zonelayerstate.NewCharacter(2, "Binh", 2))
	for range 3 {
		c.ZoneLayer(1).Send(command.NormalAttackZoneLayerCommand{CharID: 1, TargetID: 2})
		steps(c, 8)
	}
	if ch, _ := c.ZoneLayer(1).State().CharacterByID(2); ch.Health != 0 {
		t.Fatalf("the target has %d health, want it dead", ch.Health)
	}
}

func TestACronHealsEveryCharacterEveryPeriod(t *testing.T) {
	c, _, _, _ := newWorld(t, 1, 10)
	hurt := zonelayerstate.NewCharacter(1, "An", 100)
	hurt.Health = 50
	enterCharacter(t, c, 1, hurt)
	before, _ := c.ZoneLayer(1).State().CharacterByID(1)
	steps(c, 100)
	after, _ := c.ZoneLayer(1).State().CharacterByID(1)
	if after.Health <= before.Health || after.Health > after.MaxHealth {
		t.Fatalf("health went from %d to %d, want it to rise", before.Health, after.Health)
	}
}

func TestAnExitMovesTheCharacterAtOnceToAZoneLayerOfTheOtherZone(t *testing.T) {
	c, _, _, _ := newWorld(t, 1, 0)
	walkToTheEastExit(t, c, 1)
	c.ZoneLayer(1).Send(command.UseExitZoneLayerCommand{CharID: 1, ExitID: 1})
	steps(c, 4)

	if _, ok := c.ZoneLayer(1).State().CharacterByID(1); ok {
		t.Fatal("the character is still in the zonelayer it left")
	}
	e, ok := c.ZoneLayer(3).State().EntityOfCharacter(1)
	if !ok || e.Pos != (state.Vec{X: 1, Y: 2}) {
		t.Fatalf("the character is not at the arrival point of zone 2: %+v, %v", e, ok)
	}
}

func TestAnExitDoesNothingWhenTheOtherZoneHasNoRoom(t *testing.T) {
	c, _, _, _ := newWorld(t, 1, 0)
	enter(t, c, 2, 9, "Full") // zone 2 takes one character, so it is now full
	walkToTheEastExit(t, c, 1)
	c.ZoneLayer(1).Send(command.UseExitZoneLayerCommand{CharID: 1, ExitID: 1})
	steps(c, 6)

	if _, ok := c.ZoneLayer(1).State().CharacterByID(1); !ok {
		t.Fatal("the character left a zone that it had no room to go to")
	}
	if n := c.ZoneLayer(3).State().CharacterCount(); n != 1 {
		t.Fatalf("zonelayer 3 holds %d characters, want only the one that was there", n)
	}
}

func TestAnExitToAZoneThatIsNotBuiltDoesNothing(t *testing.T) {
	exit := tilemap.Exit{ID: 1, Min: state.Vec{X: 9, Y: 0}, Max: state.Vec{X: 9, Y: 4}, ToZoneID: 77, Arrival: state.Vec{X: 1, Y: 1}}
	c, err := engine.NewBuilder().
		WithSeed(1).
		WithWorld(engine.WorldParams{ID: 1}).
		WithZone(engine.ZoneParams{ID: 1, Tiles: tilemap.New(10, 5, state.Vec{X: 1, Y: 2}, nil, []tilemap.Exit{exit})}).
		WithZoneLayers(engine.ZoneLayerParams{ZoneID: 1, Count: 1, Capacity: 4}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	walkToTheEastExit(t, c, 1)
	c.ZoneLayer(1).Send(command.UseExitZoneLayerCommand{CharID: 1, ExitID: 1})
	steps(c, 6)

	if _, ok := c.ZoneLayer(1).State().CharacterByID(1); !ok {
		t.Fatal("the character is lost")
	}
}

func TestACharacterWhoseEnterCannotBeDeliveredIsPutBackWhereItWas(t *testing.T) {
	c, _, _, _ := newWorld(t, 1, 0)
	walkToTheEastExit(t, c, 1)
	// The zonelayer that the exit will choose goes away after it published how many characters it
	// holds, so the exit sees room and the message to it cannot be delivered.
	c.Env.Router.Retire(action.ZoneLayerAddress(3))
	before, _ := c.ZoneLayer(1).State().EntityOfCharacter(1)

	c.ZoneLayer(1).Send(command.UseExitZoneLayerCommand{CharID: 1, ExitID: 1})
	steps(c, 10)

	after, ok := c.ZoneLayer(1).State().EntityOfCharacter(1)
	if !ok || after.Pos != before.Pos {
		t.Fatalf("the character was not put back where it was: %+v, %v (was %+v)", after, ok, before.Pos)
	}
	if _, ok := c.ZoneLayer(3).State().CharacterByID(1); ok {
		t.Fatal("the character is in the zonelayer that was gone")
	}
}

func TestEachActorRecordsTheHashOfItsStateAtEveryNthTickFromItsOwnStart(t *testing.T) {
	c, _, j, cfg := newWorld(t, 1, 0)
	if _, err := cfg.Apply(map[string]string{"replay_log.hash_every_ticks": "5"}); err != nil {
		t.Fatal(err)
	}
	steps(c, 22)
	ticks := map[action.Address][]uint64{}
	for _, r := range j.Records {
		if r.Kind == engine.RecordHash {
			ticks[r.Engine] = append(ticks[r.Engine], r.Tick)
		}
	}
	if len(ticks) == 0 {
		t.Fatal("no hash was recorded")
	}
	starts := map[uint64]bool{}
	for who, at := range ticks {
		for i := 1; i < len(at); i++ {
			if at[i]-at[i-1] != 5 {
				t.Fatalf("%v recorded at ticks %v, want every 5th", who, at)
			}
		}
		starts[at[0]%5] = true
	}
	if len(starts) < 2 {
		t.Fatal("every engine started its count at the same tick: the hashes make a peak")
	}
}

func TestTheSameInputsGiveTheSameReplayLog(t *testing.T) {
	run := func() []engine.Record {
		c, _, j, _ := newWorld(t, 7, 0)
		enter(t, c, 1, 1, "An")
		enter(t, c, 1, 2, "Binh")
		c.ZoneLayer(1).Send(command.MoveZoneLayerCommand{CharID: 1, Dir: state.East, Ticks: 5})
		c.ZoneLayer(1).Send(command.NormalAttackZoneLayerCommand{CharID: 2, TargetID: 1})
		c.Zone(1).Send(command.ChatZoneCommand{FromID: 1, Text: "hello"})
		steps(c, 15)
		return j.Records
	}
	a, b := run(), run()
	if len(a) == 0 || !slices.EqualFunc(a, b, func(x, y engine.Record) bool {
		return x.Kind == y.Kind && x.Engine == y.Engine && x.Tick == y.Tick && x.Value == y.Value && x.Header == y.Header
	}) {
		t.Fatal("two runs of the same inputs recorded different replay logs")
	}
}

func TestActorsRunOnTheirOwnGoroutinesWithoutWaitingForEachOther(t *testing.T) {
	c, _, _, _ := newWorld(t, 1, 0)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx, time.Millisecond); close(done) }()

	c.Zone(1).Send(command.EnterWorldZoneCommand{Character: zonelayerstate.NewCharacter(1, "An", 100)})
	c.Zone(2).Send(command.EnterWorldZoneCommand{Character: zonelayerstate.NewCharacter(2, "Binh", 100)})

	// Read only what the engines published: their own states belong to their goroutines.
	deadline := time.Now().Add(5 * time.Second)
	for {
		view := c.Env.Stable.View(action.ZoneAddress(1), engine.NopReplayLog{}) // a new view each time: it keeps what it reads
		first, other := view.ZoneLayer(1), view.ZoneLayer(3)
		if first != nil && other != nil && first.CharacterCount() == 1 && other.CharacterCount() == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the characters did not enter the world")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
}
