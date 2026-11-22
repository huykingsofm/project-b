package zonelayer_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state/entity"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer/event"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap/tilemaptest"
	"github.com/huykingsofm/project-b/server/internal/shared/aliasing"
)

// tile is the width of a tile in units; the map keeps it private, so a test that wants to name a
// place by its tile says it here.
const tile = 1024

const cooldownKey zonelayer.CooldownKey = 1

func newState() *zonelayer.State { return zonelayer.NewState(1, 1) }

func map10x5(t testing.TB) *tilemap.TileMap {
	return tilemaptest.New(t, 10, 5, tilemap.Tile{X: 1, Y: 2}, nil, nil)
}

func TestAnItemAppearsAndDisappearsAndTheEventsSayIt(t *testing.T) {
	st := newState()
	id := st.AddItemEntity(7, 1, state.Vec{X: 3500, Y: 3500}, 0)
	item, ok := st.ItemEntityByID(id)
	if !ok || item.ItemID != 7 || item.Pos != (state.Vec{X: 3500, Y: 3500}) {
		t.Fatalf("ItemEntityByID = %+v, %v", item, ok)
	}
	if _, ok := st.RemoveItemEntity(id); !ok {
		t.Fatal("the item could not be removed")
	}
	if _, ok := st.RemoveItemEntity(id); ok {
		t.Fatal("an item was removed twice")
	}
	if _, ok := st.EntityByID(id); ok {
		t.Fatal("the item is still in the zonelayer")
	}
	events := st.TakeEvents()
	if len(events) != 2 || events[0].ID() != event.ItemAppearedID ||
		events[1].ID() != event.ItemDisappearedID {
		t.Fatalf("events = %+v, want appeared then disappeared", events)
	}
}

func TestAnyEntityCanBeMovedIncludingAnItem(t *testing.T) {
	st := newState()
	tiles := map10x5(t)
	item := st.AddItemEntity(7, 1, state.Vec{X: 3500, Y: 3500}, 0)
	if !st.MoveEntity(tiles, item, state.East, 2000) {
		t.Fatal("an item did not move")
	}
	if e, _ := st.ItemEntityByID(item); e.Pos != (state.Vec{X: 5500, Y: 3500}) {
		t.Fatalf("the item is at %+v, want the middle of tile (5,3)", e.Pos)
	}
	if st.MoveEntity(tiles, 999, state.East, 1000) {
		t.Fatal("an entity that does not exist moved")
	}
}

func TestACharacterIsNotRemovedAsAnItem(t *testing.T) {
	st := newState()
	id := st.AddCharacterEntity(newCharacter(1, "a", 10, 1000, 0), state.Vec{X: 1500, Y: 1500})
	if _, ok := st.RemoveItemEntity(id); ok {
		t.Fatal("a character was removed as an item")
	}
}

func TestTheHealthOfACharacterLivesOnItsBodyAndTravelsWithIt(t *testing.T) {
	st := newState()
	id := st.AddCharacterEntity(newCharacter(1, "a", 10, 1000, 0), state.Vec{X: 1500, Y: 1500})
	if died := st.DamageEntity(id, 4); died {
		t.Fatal("4 damage killed a character with 10 health")
	}
	if c, _ := st.CharacterByID(1); c.Health != 6 || c.MaxHealth != 10 {
		t.Fatalf("health = %d of %d, want 6 of 10", c.Health, c.MaxHealth)
	}
	st.HealAllEntities(100)
	if c, _ := st.CharacterByID(1); c.Health != 10 {
		t.Fatalf("health after a heal = %d, want 10", c.Health)
	}
	st.DamageEntity(id, 3)
	out, _ := st.RemoveCharacterEntity(1)
	if out.Health != 7 || out.MaxHealth != 10 {
		t.Fatalf("the character left with %d of %d health, want 7 of 10", out.Health, out.MaxHealth)
	}
	back := st.AddCharacterEntity(out, state.Vec{X: 2500, Y: 2500})
	if body, _ := st.CharacterEntityByCharacterID(1); body.EntityID != back || body.Hp != 7 {
		t.Fatalf("the body that came back has %d health, want 7", body.Hp)
	}
}

func TestOnlyADamageableEntityLosesHealth(t *testing.T) {
	st := newState()
	item := st.AddItemEntity(7, 1, state.Vec{X: 3500, Y: 3500}, 0)
	if st.DamageEntity(item, 5) {
		t.Fatal("an item that cannot be damaged died")
	}
	if _, ok := st.ItemEntityByID(item); !ok {
		t.Fatal("damage removed an item")
	}
}

// R29: nothing in a state is shared with its clone.
func TestAChangeAfterCloneDoesNotChangeTheCopyAndTheOtherWayRound(t *testing.T) {
	st := newState()
	st.AddCharacterEntity(newCharacter(1, "a", 10, 1000, 0), state.Vec{X: 1500, Y: 1500})
	st.SetCooldown(1, cooldownKey, 5)
	st.AddItemEntity(7, 1, state.Vec{X: 3500, Y: 3500}, 0)
	st.SendMessage(zonelayer.ChatMessage{FromID: 1, Text: "hi"})

	before := st.Hash()
	chatBefore := st.Chat.Len()
	copied := st.Clone()

	st.SetCooldown(1, cooldownKey, 99)
	st.DamageEntity(1, 3)
	st.AddItemEntity(8, 1, state.Vec{X: 4500, Y: 4500}, 0)
	st.SendMessage(zonelayer.ChatMessage{FromID: 1, Text: "again"})
	if copied.Hash() != before || copied.Chat.Len() != chatBefore {
		t.Fatal("a change of the state changed its copy")
	}

	copiedHash := copied.Hash()
	copied.SetCooldown(1, cooldownKey, 7)
	copied.RemoveItemEntity(2)
	if st.Hash() == copiedHash {
		t.Fatal("the test did not change the state apart from the copy")
	}
	if until := st.CooldownUntil(1, cooldownKey); until != 99 {
		t.Fatalf("a change of the copy changed the state: cooldown = %d", until)
	}
	if _, ok := st.ItemEntityByID(2); !ok {
		t.Fatal("removing an item from the copy removed it from the state")
	}
}

func TestAMoveStopsAtTheLastUnitBeforeAWallAndCanBeShorterThanATile(t *testing.T) {
	walls := tilemaptest.New(t, 10, 5, tilemap.Tile{X: 1, Y: 2}, []tilemap.Tile{{X: 4, Y: 3}}, nil)
	st := newState()
	id := st.AddItemEntity(7, 1, state.Vec{X: 2*tile + tile/2, Y: 3*tile + tile/2}, 0)
	// The item is in the middle of tile (2,3); the wall is tile (4,3), which starts a tile and a
	// half away.
	if !st.MoveEntity(walls, id, state.East, 5*tile) {
		t.Fatal("the item did not move")
	}
	if e, _ := st.ItemEntityByID(id); e.Pos != (state.Vec{X: 4*tile - 1, Y: 3*tile + tile/2}) {
		t.Fatalf("the item stopped at %+v, want the last unit before the wall", e.Pos)
	}
	if !st.MoveEntity(walls, id, state.South, tile/4) {
		t.Fatal("a move of a quarter of a tile did not move")
	}
	if e, _ := st.ItemEntityByID(
		id,
	); e.Pos != (state.Vec{X: 4*tile - 1, Y: 3*tile + tile/2 + tile/4}) {
		t.Fatalf("the item is at %+v, want it a quarter of a tile south", e.Pos)
	}
}

func TestTheRadiusOfACharacterLivesOnItsBodyAndTravelsWithIt(t *testing.T) {
	st := newState()
	id := st.AddCharacterEntity(
		newCharacter(1, "a", 10, 1000, 300), state.Vec{X: tile + tile/2, Y: tile + tile/2},
	)
	body, _ := st.CharacterEntityByCharacterID(1)
	if body.Rad != 300 || body.ID() != id {
		t.Fatalf("body = %+v, want a radius of 300", body)
	}
	if c, _ := st.CharacterByID(1); c.Radius != 300 {
		t.Fatalf("the character says a radius of %d, want 300", c.Radius)
	}
	c, _ := st.RemoveCharacterEntity(1)
	if c.Radius != 300 {
		t.Fatalf("the character that left has a radius of %d, want 300", c.Radius)
	}
}

func TestABodyMovesOnlyAsFarAsItsRadiusAllows(t *testing.T) {
	walls := tilemaptest.New(t, 10, 5, tilemap.Tile{X: 1, Y: 2}, []tilemap.Tile{{X: 4, Y: 3}}, nil)
	st := newState()
	at := state.Vec{X: 2 * tile, Y: 2*tile + tile/2}
	point := st.AddCharacterEntity(newCharacter(1, "point", 10, 1000, 0), at)
	body := st.AddCharacterEntity(newCharacter(2, "body", 10, 1000, 600), at)

	st.MoveEntity(walls, point, state.East, 9*tile)
	st.MoveEntity(walls, body, state.East, 9*tile)
	p, _ := st.CharacterEntityByCharacterID(1)
	b, _ := st.CharacterEntityByCharacterID(2)
	if p.Pos.X != 10*tile-1 {
		t.Fatalf("the point is at %+v, want it to pass the wall beside its path", p.Pos)
	}
	if b.Pos.X != 4*tile-1-600 {
		t.Fatalf("the body is at %+v, want it stopped by the wall beside its path", b.Pos)
	}
}

func BenchmarkMoveEntity(b *testing.B) {
	tiles := tilemaptest.New(b, 200, 200, tilemap.Tile{}, nil, nil)
	for _, radius := range []int32{0, 256} {
		b.Run(fmt.Sprintf("radius %d/a character's step of a tick", radius), func(b *testing.B) {
			st := newState()
			id := st.AddCharacterEntity(
				newCharacter(1, "a", 10, 1000, radius), state.Vec{X: 100 * tile, Y: 100 * tile},
			)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				dir := state.East
				if i%2 == 1 {
					dir = state.West
				}
				st.MoveEntity(tiles, id, dir, 250)
				st.TakeEvents()
			}
		})
	}
}

// newCharacter is a character of the values of a test, which are never invalid.
func newCharacter(
	characterID state.CharacterID, name string, maxHealth, speed, radius int32,
) zonelayer.Character {
	c, err := zonelayer.NewCharacter(characterID, name, maxHealth, speed, radius)
	if err != nil {
		panic(err)
	}
	return c
}

func TestACharacterWithANegativeRadiusIsAnErrorNotACharacter(t *testing.T) {
	if _, err := zonelayer.NewCharacter(1, "a", 10, 1000, -1); err == nil {
		t.Fatal("a negative radius was accepted")
	}
}

// R29: a character that enters is copied, so the state shares nothing with what the caller sent and
// still holds.
func TestACharacterThatEntersSharesNothingWithTheStateItEnters(t *testing.T) {
	st := newState()
	c := newCharacter(1, "a", 10, 1000, 0)
	c.Cooldowns.Set(cooldownKey, 5)
	st.AddCharacterEntity(c, state.Vec{X: 1500, Y: 1500})
	stored, _ := st.Characters.Get(1)
	if err := aliasing.Disjoint(c, stored); err != nil {
		t.Fatal(err)
	}
	st.SetCooldown(1, cooldownKey, 99)
	if got, _ := c.Cooldowns.Get(cooldownKey); got != 5 || c.Cooldowns.Len() != 1 {
		t.Fatalf(
			"the state changed the character that was sent: %d, %d cooldowns",
			got,
			c.Cooldowns.Len(),
		)
	}
}

// R29: a character that leaves is the one the state held, now outside it; what it gives up is not
// shared with a copy that was published.
func TestAClonedStateSharesNoMemoryWithTheState(t *testing.T) {
	st := newState()
	c := newCharacter(1, "a", 10, 1000, 0)
	c.Cooldowns.Set(cooldownKey, 5)
	st.AddCharacterEntity(c, state.Vec{X: 1500, Y: 1500})
	st.AddItemEntity(7, 1, state.Vec{X: 3500, Y: 3500}, 0)
	st.SendMessage(zonelayer.ChatMessage{FromID: 1, Text: "hi"})
	if err := aliasing.Disjoint(st, st.Clone()); err != nil {
		t.Fatal(err)
	}
}

func TestTheTypesOfTheZoneLayerStateObeyTheRuleOfData(t *testing.T) {
	// an Entity is implemented only by value types
	rules := aliasing.Rules{
		Interfaces: []reflect.Type{reflect.TypeFor[entity.Entity]()},
		Skip:       []string{"zonelayer.State.events"},
	}
	if err := aliasing.CheckCloneable(reflect.TypeFor[zonelayer.State](), rules); err != nil {
		t.Fatal(err)
	}
}
