package game_test

import (
	"context"
	"github.com/huykingsofm/project-b/server/game/sim/state/zonelayer/entity"
	"slices"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/server/game"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/command"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	worldevent "github.com/huykingsofm/project-b/server/game/sim/state/world/event"
	zoneevent "github.com/huykingsofm/project-b/server/game/sim/state/zone/event"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	zonelayerevent "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer/event"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap/tilemaptest"
	"github.com/huykingsofm/project-b/server/game/testkit"
)

// tile is the width of a tile in units; the map keeps it private, so a test that wants to name a
// place by its tile says it here.
const tile = 1024

func TestEnteringTheWorldPutsTheCharacterInAZoneLayer(t *testing.T) {
	c, rec, _, _ := testkit.NewWorld(t, 1, 0)
	testkit.Enter(t, c, testkit.Zone1, 1, "An")

	if _, ok := c.ZoneLayer(testkit.Layer1).State().CharacterEntityByCharacterID(1); !ok {
		t.Fatal("the character has no body in zonelayer 1")
	}
	if n := rec.Count[zonelayerevent.CharacterEntered](); n != 1 {
		t.Fatalf("the zonelayer told %d characters entered, want 1", n)
	}
}

func TestAFullZoneLayerSendsTheNextCharacterToAnotherZoneLayer(t *testing.T) {
	c, _, _, _ := testkit.NewWorld(t, 1, 0)
	testkit.Enter(t, c, testkit.Zone1, 1, "An")
	testkit.Enter(t, c, testkit.Zone1, 2, "Binh")
	// zonelayer 1 holds two characters, so the third goes to zonelayer 2
	testkit.Enter(t, c, testkit.Zone1, 3, "Chi")
	if _, ok := c.ZoneLayer(testkit.Layer2).State().CharacterByID(3); !ok {
		t.Fatal("the third character is not in zonelayer 2")
	}
}

func TestRunsOfMovementAreAppliedInOrderThroughTheCooldown(t *testing.T) {
	c, _, _, _ := testkit.NewWorld(t, 1, 0)
	testkit.Enter(t, c, testkit.Zone1, 1, "An")
	l := c.ZoneLayer(testkit.Layer1)
	start, _ := l.State().CharacterEntityByCharacterID(1)

	l.Send(command.ZoneLayerCharacterMoveCommand{CharacterID: 1, Dir: state.East, Ticks: 3})
	// arrives while the first runs: it waits its turn
	l.Send(command.ZoneLayerCharacterMoveCommand{CharacterID: 1, Dir: state.South, Ticks: 2})
	testkit.Steps(c, 8)

	end, _ := l.State().CharacterEntityByCharacterID(1)
	at := start.Position()
	if want := (state.Vec{X: at.X + 3000, Y: at.Y + 2000}); end.Position() != want {
		t.Fatalf("position = %+v, want %+v", end.Position(), want)
	}
	if until := l.State().CooldownUntil(
		1, action.ZoneLayerCharacterMoveActionCooldownKey,
	); until == 0 {
		t.Fatal("the movement left no cooldown")
	}
}

func TestChatInEachScope(t *testing.T) {
	c, rec, _, _ := testkit.NewWorld(t, 1, 0)
	testkit.Enter(t, c, testkit.Zone1, 1, "An")

	c.ZoneLayer(testkit.Layer1).Send(command.ZoneLayerChatSendCommand{FromID: 1, Text: "zonelayer"})
	testkit.Steps(c, 3)
	if n := rec.Count[zonelayerevent.ChatMessageSent](); n != 1 {
		t.Fatalf("a zonelayer message was heard by %d zonelayers, want 1", n)
	}
	if n := rec.Count[zoneevent.ChatMessageSent](); n != 0 {
		t.Fatalf("a zonelayer message reached the zone %d times", n)
	}

	c.Zone(testkit.Zone1).Send(command.ZoneChatSendCommand{FromID: 1, Text: "zone"})
	testkit.Steps(c, 3)
	if n := rec.Count[zoneevent.ChatMessageSent](); n != 1 {
		t.Fatalf("the zone recorded %d messages, want 1", n)
	}
	// the first, and one for each of the two zonelayers of zone 1
	if n := rec.Count[zonelayerevent.ChatMessageSent](); n != 3 {
		t.Fatalf("zonelayer messages heard = %d, want 3", n)
	}

	c.World.Send(command.WorldChatPrivateSendCommand{FromID: 10, ToID: 20, Text: "private"})
	testkit.Steps(c, 3)
	if n := rec.Count[worldevent.PrivateChatMessageSent](); n != 1 {
		t.Fatalf("the world recorded %d private messages, want 1", n)
	}
	if n := rec.Count[zonelayerevent.ChatMessageSent](); n != 3 {
		t.Fatalf("a private message reached a zonelayer: %d zonelayer messages", n)
	}
}

func TestNormalAttackCooldownAndShield(t *testing.T) {
	c, _, _, _ := testkit.NewWorld(t, 1, 0)
	testkit.Enter(t, c, testkit.Zone1, 1, "An")
	testkit.Enter(t, c, testkit.Zone1, 2, "Binh")
	l := c.ZoneLayer(testkit.Layer1)
	health := func(characterID state.CharacterID) int32 {
		ch, _ := l.State().CharacterByID(characterID)
		return ch.Health()
	}

	l.Send(command.ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID: 1, TargetID: 2})
	testkit.Steps(c, 3)
	if health(2) != 99 {
		t.Fatalf("health after a hit = %d, want 99", health(2))
	}

	// The attack is on its cooldown: the second one does nothing.
	l.Send(command.ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID: 1, TargetID: 2})
	testkit.Steps(c, 2)
	if health(2) != 99 {
		t.Fatalf("an attack in the cooldown did something: health %d", health(2))
	}

	// After the cooldown the attack lands again.
	testkit.Steps(c, 6)
	l.Send(command.ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID: 1, TargetID: 2})
	testkit.Steps(c, 3)
	if health(2) != 98 {
		t.Fatalf("the attack after the cooldown did not land: health %d, want 98", health(2))
	}

	// A shield stands in the plan, runs before the damage lands, and blocks the first one only.
	l.Send(command.ZoneLayerCharacterShieldGrantCommand{CharacterID: 2, Duration: 1000})
	testkit.Steps(c, 8)
	l.Send(command.ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID: 1, TargetID: 2})
	testkit.Steps(c, 3)
	if health(2) != 98 {
		t.Fatalf("a shield did not block a hit: health %d", health(2))
	}
	testkit.Steps(c, 8)
	l.Send(command.ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID: 1, TargetID: 2})
	testkit.Steps(c, 3)
	if health(2) != 97 {
		t.Fatalf("the shield blocked a second hit: health %d, want 97", health(2))
	}
}

func TestAttackOnATargetThatIsNotThereDoesNothing(t *testing.T) {
	c, _, _, _ := testkit.NewWorld(t, 1, 0)
	testkit.Enter(t, c, testkit.Zone1, 1, "An")
	l := c.ZoneLayer(testkit.Layer1)
	l.Send(command.ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID: 1, TargetID: 9})
	testkit.Steps(c, 3)
	if ch, _ := l.State().CharacterByID(1); ch.Health() != 100 {
		t.Fatalf("an attack on nobody changed health to %d", ch.Health())
	}
}

func TestAKilledCharacterStaysDead(t *testing.T) {
	c, _, _, _ := testkit.NewWorld(t, 1, 0)
	testkit.Enter(t, c, testkit.Zone1, 1, "An")
	testkit.EnterCharacter(t, c, testkit.Zone1, testkit.NewCharacter(2, "Binh", 2, 1000, 0))
	for range 3 {
		c.ZoneLayer(testkit.Layer1).Send(
			command.ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID: 1, TargetID: 2},
		)
		testkit.Steps(c, 8)
	}
	if ch, _ := c.ZoneLayer(testkit.Layer1).State().CharacterByID(2); ch.Health() != 0 {
		t.Fatalf("the target has %d health, want it dead", ch.Health())
	}
}

func TestAZoneLayerHealsEveryCharacterEveryPeriod(t *testing.T) {
	c, _, _, _ := testkit.NewWorld(t, 1, 10)
	hurt := testkit.NewCharacter(1, "An", 100, 1000, 0)
	hurt.SetHealth(50)
	testkit.EnterCharacter(t, c, testkit.Zone1, hurt)
	before, _ := c.ZoneLayer(testkit.Layer1).State().CharacterByID(1)
	testkit.Steps(c, 100)
	after, _ := c.ZoneLayer(testkit.Layer1).State().CharacterByID(1)
	if after.Health() <= before.Health() || after.Health() > after.MaxHealth() {
		t.Fatalf("health went from %d to %d, want it to rise", before.Health(), after.Health())
	}
}

func TestAnExitMovesTheCharacterAtOnceToAZoneLayerOfTheOtherZone(t *testing.T) {
	c, _, _, _ := testkit.NewWorld(t, 1, 0)
	testkit.WalkToTheEastExit(t, c, 1)
	c.ZoneLayer(testkit.Layer1).Send(
		command.ZoneLayerCharacterExitToCommand{CharacterID: 1, ExitEntityID: testkit.EastExit},
	)
	testkit.Steps(c, 4)

	if _, ok := c.ZoneLayer(testkit.Layer1).State().CharacterByID(1); ok {
		t.Fatal("the character is still in the zonelayer it left")
	}
	e, ok := c.ZoneLayer(testkit.Layer3).State().CharacterEntityByCharacterID(1)
	if !ok || e.Position() != (state.Vec{X: tile + tile/2, Y: 2*tile + tile/2}) {
		t.Fatalf("the character is not at the arrival point of zone 2: %+v, %v", e, ok)
	}
}

func TestAnExitDoesNothingWhenTheOtherZoneHasNoRoom(t *testing.T) {
	c, _, _, _ := testkit.NewWorld(t, 1, 0)
	testkit.Enter(t, c, testkit.Zone2, 9, "Full") // zone 2 takes one character, so it is now full
	testkit.WalkToTheEastExit(t, c, 1)
	c.ZoneLayer(testkit.Layer1).Send(
		command.ZoneLayerCharacterExitToCommand{CharacterID: 1, ExitEntityID: testkit.EastExit},
	)
	testkit.Steps(c, 6)

	if _, ok := c.ZoneLayer(testkit.Layer1).State().CharacterByID(1); !ok {
		t.Fatal("the character left a zone that it had no room to go to")
	}
	if n := c.ZoneLayer(testkit.Layer3).State().CharacterCount(); n != 1 {
		t.Fatalf("zonelayer 3 holds %d characters, want only the one that was there", n)
	}
}

func TestAnExitToAZoneThatIsNotBuiltDoesNothing(t *testing.T) {
	exit := tilemap.ExitDefinition{
		Min:      tilemap.Tile{X: 9, Y: 0},
		Max:      tilemap.Tile{X: 9, Y: 4},
		ToZoneID: 77,
		Arrival:  tilemap.Tile{X: 1, Y: 1},
	}
	c, err := game.NewBuilder().
		WithSeed(1).
		WithWorld(game.WorldParams{ID: testkit.World}).
		WithZone(
			game.ZoneParams{
				ID: testkit.Zone1,
				Tiles: tilemaptest.New(
					t, 10, 5, tilemap.Tile{X: 1, Y: 2}, nil, []tilemap.ExitDefinition{exit},
				),
			},
		).
		WithZone(
			game.ZoneParams{
				ID:    testkit.Zone2,
				Tiles: tilemaptest.New(t, 10, 5, tilemap.Tile{X: 1, Y: 2}, nil, nil),
			},
		).
		WithZoneLayers(game.ZoneLayerParams{ZoneID: testkit.Zone1, Count: 1, Capacity: 4}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	testkit.WalkToTheEastExit(t, c, 1)
	c.ZoneLayer(testkit.Layer1).Send(
		command.ZoneLayerCharacterExitToCommand{CharacterID: 1, ExitEntityID: testkit.EastExit},
	)
	testkit.Steps(c, 6)

	if _, ok := c.ZoneLayer(testkit.Layer1).State().CharacterByID(1); !ok {
		t.Fatal("the character is lost")
	}
}

func TestACharacterWhoseEnterCannotBeDeliveredIsPutBackWhereItWas(t *testing.T) {
	c, _, _, _ := testkit.NewWorld(t, 1, 0)
	testkit.WalkToTheEastExit(t, c, 1)
	// The zonelayer that the exit will choose goes away after it published how many characters it
	// holds, so the exit sees room and the message to it cannot be delivered.
	c.Env.Router.Retire(testkit.Layer3)
	before, _ := c.ZoneLayer(testkit.Layer1).State().CharacterEntityByCharacterID(1)

	c.ZoneLayer(testkit.Layer1).Send(
		command.ZoneLayerCharacterExitToCommand{CharacterID: 1, ExitEntityID: testkit.EastExit},
	)
	testkit.Steps(c, 10)

	after, ok := c.ZoneLayer(testkit.Layer1).State().CharacterEntityByCharacterID(1)
	if !ok || after.Position() != before.Position() {
		t.Fatalf(
			"the character was not put back where it was: %+v, %v (was %+v)",
			after, ok, before.Position(),
		)
	}
	if _, ok := c.ZoneLayer(testkit.Layer3).State().CharacterByID(1); ok {
		t.Fatal("the character is in the zonelayer that was gone")
	}
}

func TestAMessageThatCameBackIsRecordedWithItsHeaderAndReasonAtTheTickItWasSettled(
	t *testing.T,
) {
	c, _, j, _ := testkit.NewWorld(t, 1, 0)
	testkit.WalkToTheEastExit(t, c, 1)
	c.Env.Router.Retire(testkit.Layer3)

	c.ZoneLayer(testkit.Layer1).Send(
		command.ZoneLayerCharacterExitToCommand{CharacterID: 1, ExitEntityID: testkit.EastExit},
	)
	testkit.Steps(c, 10)

	var returned []replay.Record
	for _, r := range j.Records {
		if r.Kind() == replay.KindReturned {
			returned = append(returned, r)
		}
	}
	if len(returned) != 1 {
		t.Fatalf("%d records of a message that came back, want 1", len(returned))
	}
	got := returned[0]
	body := got.Body.(replay.Returned)
	layer := testkit.Layer1
	if got.Engine != layer || body.Header.From != layer ||
		body.Header.Seq == 0 || body.Reason != sim.Retired {
		t.Fatalf("the record is %+v, want zonelayer 1's own message, returned as Retired", got)
	}
	var settledAt uint64
	for _, r := range j.Records {
		inbound, ok := r.Body.(replay.Inbound)
		if ok && r.Engine == testkit.Layer1 &&
			inbound.Message.Head().From == testkit.Layer1 && r.Tick > got.Tick {
			settledAt = r.Tick // the compensation, which is sent to itself, is taken after
		}
	}
	if settledAt == 0 {
		t.Fatal("the compensation that follows the return was not taken at a later tick")
	}
}

func TestASecondCopyOfAMessageIsDroppedAndAMessageWithNoNumberIsNeverACopy(t *testing.T) {
	c, _, j, _ := testkit.NewWorld(t, 1, 0)
	layer := testkit.Layer1
	say := func(seq uint64) sim.Message {
		m := sim.NewActionMessage[*zonelayerstate.State](
			testkit.Layer1, onfail.Drop{}, &action.ZoneLayerChatSendAction{FromID: 1, Text: "hi"},
		)
		return m.Stamp(sim.Header{From: testkit.Zone1, Seq: seq})
	}
	for _, seq := range []uint64{5, 5, 4, 6} { // a copy, a number that went back, and a new one
		if _, ok := c.Env.Router.Deliver(say(seq)); !ok {
			t.Fatal("the message was not delivered")
		}
	}
	testkit.Steps(c, 1)
	if got := c.ZoneLayer(testkit.Layer1).State().ChatMessageCount(); got != 2 {
		t.Fatalf("%d chat lines, want 2: the numbers 5 and 6, once each", got)
	}
	for range 2 { // no number: two messages, both taken
		c.Env.Router.Deliver(say(0))
	}
	testkit.Steps(c, 1)
	if got := c.ZoneLayer(testkit.Layer1).State().ChatMessageCount(); got != 4 {
		t.Fatalf("%d chat lines, want 4: the two with no number are not copies", got)
	}
	taken := 0
	for _, r := range j.Records {
		if r.Kind() == replay.KindInbound && r.Engine == layer {
			taken++
		}
	}
	if taken != 4 {
		t.Fatalf("%d messages recorded as taken by the zonelayer, want 4 (no copy)", taken)
	}
}

func TestEachActorRecordsTheHashOfItsStateAtEveryNthTickFromItsOwnStart(t *testing.T) {
	c, _, j, cfg := testkit.NewWorld(t, 1, 0)
	if _, err := cfg.Apply(map[string]string{"replay_log.hash_every_ticks": "5"}); err != nil {
		t.Fatal(err)
	}
	testkit.Steps(c, 22)
	ticks := map[sim.StateID][]uint64{}
	for _, r := range j.Records {
		if r.Kind() == replay.KindHash {
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
	run := func() []replay.Record {
		c, _, j, _ := testkit.NewWorld(t, 7, 0)
		testkit.Enter(t, c, testkit.Zone1, 1, "An")
		testkit.Enter(t, c, testkit.Zone1, 2, "Binh")
		c.ZoneLayer(testkit.Layer1).Send(
			command.ZoneLayerCharacterMoveCommand{CharacterID: 1, Dir: state.East, Ticks: 5},
		)
		c.ZoneLayer(testkit.Layer1).Send(
			command.ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID: 2, TargetID: 1},
		)
		c.Zone(testkit.Zone1).Send(command.ZoneChatSendCommand{FromID: 1, Text: "hello"})
		testkit.Steps(c, 15)
		return j.Records
	}
	a, b := run(), run()
	if len(a) == 0 || !slices.EqualFunc(a, b, func(x, y replay.Record) bool {
		return x.Engine == y.Engine && x.Tick == y.Tick && x.Body.Same(y.Body)
	}) {
		t.Fatal("two runs of the same inputs recorded different replay logs")
	}
}

func TestActorsRunOnTheirOwnGoroutinesWithoutWaitingForEachOther(t *testing.T) {
	c, _, _, _ := testkit.NewWorld(t, 1, 0)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx, time.Millisecond); close(done) }()

	an := testkit.NewCharacter(1, "An", 100, 1000, 0)
	binh := testkit.NewCharacter(2, "Binh", 100, 1000, 0)
	c.Zone(testkit.Zone1).Send(command.ZoneCharacterEnterCommand{Character: an})
	c.Zone(testkit.Zone2).Send(command.ZoneCharacterEnterCommand{Character: binh})

	// Read only what the engines published: their own states belong to their goroutines.
	deadline := time.Now().Add(5 * time.Second)
	for {
		// a new view each time: it keeps what it reads
		view := c.Env.Stable.View(testkit.Zone1, replay.NopLog{})
		if layerCount(view, testkit.Layer1) == 1 && layerCount(view, testkit.Layer3) == 1 {
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

func TestAPickedUpItemLeavesTheGroundAndArrivesInTheInventoryInTheWorld(t *testing.T) {
	c, rec, _, _ := testkit.NewWorld(t, 1, 0)
	testkit.Enter(t, c, testkit.Zone1, 1, "An")
	itemID := c.ZoneLayer(testkit.Layer1).State().
		AddItemEntity(7, 1, state.Vec{X: 2500, Y: 2500}, 0)

	c.ZoneLayer(testkit.Layer1).Send(
		command.ZoneLayerCharacterItemPickupCommand{CharacterID: 1, ItemEntityID: itemID},
	)
	testkit.Steps(c, 4)

	if _, ok := c.ZoneLayer(testkit.Layer1).State().EntityByID(itemID); ok {
		t.Fatal("the item is still on the ground")
	}
	if got := c.World.State().ItemsHeld(1, 7); got != 1 {
		t.Fatalf("the character has %d of the item, want 1", got)
	}
	if n := rec.Count[zonelayerevent.ItemDisappeared](); n != 1 {
		t.Fatalf("the zonelayer told %d items disappeared, want 1", n)
	}
	if n := rec.Count[worldevent.CharacterItemAdded](); n != 1 {
		t.Fatalf("the world told %d items added, want 1", n)
	}
}

func TestTwoCharactersGoingForTheSameItemAtOnceOnlyOneGetsIt(t *testing.T) {
	c, _, _, _ := testkit.NewWorld(t, 1, 0)
	testkit.Enter(t, c, testkit.Zone1, 1, "An")
	testkit.Enter(t, c, testkit.Zone1, 2, "Binh")
	itemID := c.ZoneLayer(testkit.Layer1).State().
		AddItemEntity(7, 1, state.Vec{X: 2500, Y: 2500}, 0)

	c.ZoneLayer(testkit.Layer1).Send(
		command.ZoneLayerCharacterItemPickupCommand{CharacterID: 1, ItemEntityID: itemID},
	)
	c.ZoneLayer(testkit.Layer1).Send(
		command.ZoneLayerCharacterItemPickupCommand{CharacterID: 2, ItemEntityID: itemID},
	)
	testkit.Steps(c, 4)

	got := c.World.State().ItemsHeld(1, 7) + c.World.State().ItemsHeld(2, 7)
	if got != 1 {
		t.Fatalf("%d items were handed out for one item", got)
	}
}

func TestAnItemThatDoesNotFitInTheInventoryComesBackToTheGroundWhereItWas(t *testing.T) {
	c, _, _, _ := testkit.NewWorld(t, 1, 0)
	testkit.Enter(t, c, testkit.Zone1, 1, "An")
	c.World.State().AddItemToCharacter(1, 9, 20*99, 20, 99) // every slot is full of another kind
	at := state.Vec{X: 2500, Y: 2500}
	itemID := c.ZoneLayer(testkit.Layer1).State().
		AddItemEntity(7, 3, at, 0)

	c.ZoneLayer(testkit.Layer1).Send(
		command.ZoneLayerCharacterItemPickupCommand{CharacterID: 1, ItemEntityID: itemID},
	)
	testkit.Steps(c, 6)

	if got := c.World.State().ItemsHeld(1, 7); got != 0 {
		t.Fatalf("the character got %d items that did not fit", got)
	}
	layer := c.ZoneLayer(testkit.Layer1).State()
	var back []entity.ItemEntity
	for _, id := range layer.EntityIDsOfKind(state.ItemKindID) {
		item, _ := layer.ItemEntityByID(id)
		back = append(back, item)
	}
	if len(back) != 1 || back[0].ItemID() != 7 || back[0].Count() != 3 || back[0].Position() != at {
		t.Fatalf("on the ground: %+v, want the stack of 3 of kind 7 where it was", back)
	}
}

func TestAnItemThatCameBackToTheGroundIsRemovedWhenItsExpiryEnds(t *testing.T) {
	c, _, _, _ := testkit.NewWorld(t, 1, 0)
	testkit.Enter(t, c, testkit.Zone1, 1, "An")
	c.World.State().AddItemToCharacter(1, 9, 20*99, 20, 99) // every slot is full of another kind
	itemID := c.ZoneLayer(testkit.Layer1).State().
		AddItemEntity(7, 3, state.Vec{X: 2500, Y: 2500}, 20)
	c.ZoneLayer(testkit.Layer1).Send(
		command.ZoneLayerCharacterItemPickupCommand{CharacterID: 1, ItemEntityID: itemID},
	)
	testkit.Steps(c, 6)

	layer := c.ZoneLayer(testkit.Layer1).State()
	items := func() (found []entity.ItemEntity) {
		for _, id := range layer.EntityIDs() {
			if item, ok := layer.ItemEntityByID(id); ok {
				found = append(found, item)
			}
		}
		return found
	}
	if got := items(); len(got) != 1 || got[0].Expiry() != 20 {
		t.Fatalf("on the ground: %+v, want the stack back with its expiry of 20", got)
	}
	testkit.Steps(c, 30)
	if got := items(); len(got) != 0 {
		t.Fatalf("on the ground: %+v, want the item removed when its expiry ended", got)
	}
}

func TestAnEngineDoesNotTakeANilCommand(t *testing.T) {
	c, _, _, _ := testkit.NewWorld(t, 1, 0)
	l := c.ZoneLayer(testkit.Layer1)
	if c.World.Send(nil) || c.Zone(testkit.Zone1).Send(nil) || l.Send(nil) {
		t.Fatal("an engine took a nil command")
	}
	testkit.Steps(c, 3) // and nothing in a tick breaks on it
}

// layerCount is how many characters a zonelayer published, or -1 when it published nothing.
func layerCount(view *engine.StableView, id sim.StateID) int {
	st, err := view.Read(id)
	if err != nil {
		return -1
	}
	return st.(zonelayerstate.ReadOnlyZoneLayerState).CharacterCount()
}
