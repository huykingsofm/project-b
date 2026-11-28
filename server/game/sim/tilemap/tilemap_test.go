package tilemap_test

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap/tilemaptest"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// tile is the width of a tile in units; the map keeps it private, so a test that wants to name a
// place by its tile says it here.
const tile = 1024

func TestFreeTilesAreInsideTheMapAndNotWalls(t *testing.T) {
	m := tilemaptest.New(t, 4, 3, tilemap.Tile{X: 1, Y: 1}, []tilemap.Tile{{X: 2, Y: 1}})
	for _, c := range []struct {
		at   state.Vec
		want bool
	}{
		{state.Vec{X: 500, Y: 500}, true},
		{state.Vec{X: 3*tile + tile/2, Y: 2*tile + tile/2}, true},
		{state.Vec{X: 2*tile + tile/2, Y: tile + tile/2}, false}, // a wall
		{state.Vec{X: 2*tile - 1, Y: tile + tile/2}, true},       // the last unit before the wall
		{state.Vec{X: 2 * tile, Y: tile + tile/2}, false},        // the first unit of the wall
		{state.Vec{X: -1, Y: 500}, false},                        // just outside, to the left
		{state.Vec{X: 4*tile + 500, Y: 500}, false},
		{state.Vec{X: 500, Y: 3*tile + 500}, false},
	} {
		if got := m.Free(c.at); got != c.want {
			t.Errorf("Free(%+v) = %v, want %v", c.at, got, c.want)
		}
	}
}

func TestATileIsTileSizeUnitsAndAnAreaOfTilesIsInUnitsBothEndsIncluded(t *testing.T) {
	if tilemap.TileSize != 1024 {
		t.Fatal("the size of a tile changed: it must never, the client holds it too")
	}
	middle := state.Vec{X: tile + tile/2, Y: 2*tile + tile/2}
	if got := (tilemap.Tile{X: 1, Y: 2}).Center(); got != middle {
		t.Fatalf("Center = %+v, want %+v", got, middle)
	}
	got := tilemap.AreaOf(tilemap.Tile{X: 3, Y: 0}, tilemap.Tile{X: 3, Y: 2})
	want := state.Rect{
		Min: state.Vec{X: 3 * tile, Y: 0}, Max: state.Vec{X: 4*tile - 1, Y: 3*tile - 1},
	}
	if got != want {
		t.Fatalf("AreaOf = %+v, want %+v", got, want)
	}
}

func TestAMapHoldsAnAreaThatIsNotEmptyAndInsideIt(t *testing.T) {
	m := tilemaptest.New(t, 4, 3, tilemap.Tile{}, nil)
	for name, c := range map[string]struct {
		area state.Rect
		want bool
	}{
		"the whole map": {tilemap.AreaOf(tilemap.Tile{}, tilemap.Tile{X: 3, Y: 2}), true},
		"one unit": {
			state.Rect{Min: state.Vec{X: 5, Y: 5}, Max: state.Vec{X: 5, Y: 5}}, true,
		},
		"past the east":  {tilemap.AreaOf(tilemap.Tile{X: 3}, tilemap.Tile{X: 4}), false},
		"past the south": {tilemap.AreaOf(tilemap.Tile{Y: 2}, tilemap.Tile{X: 1, Y: 3}), false},
		"before the map": {state.Rect{Min: state.Vec{X: -1}, Max: state.Vec{X: 9, Y: 9}}, false},
		"reversed": {
			state.Rect{Min: state.Vec{X: 9, Y: 9}, Max: state.Vec{X: 5, Y: 5}}, false,
		},
	} {
		if got := m.Holds(c.area); got != c.want {
			t.Errorf("%s: Holds = %v, want %v", name, got, c.want)
		}
	}
}

func TestReachStopsAtTheLastUnitBeforeAWall(t *testing.T) {
	m := tilemaptest.New(t, 10, 5, tilemap.Tile{X: 1, Y: 2}, []tilemap.Tile{{X: 4, Y: 3}})
	// the middle of tile (2,3), the wall is tile (4,3)
	from := state.Vec{X: 2*tile + tile/2, Y: 3*tile + tile/2}
	if got := m.Reach(
		from, state.East, 5*tile, 0,
	); got != (state.Vec{X: 4*tile - 1, Y: 3*tile + tile/2}) {
		t.Fatalf("Reach east = %+v, want the last unit before the wall (tile 4)", got)
	}
	if got := m.Reach(
		from, state.South, tile/4, 0,
	); got != (state.Vec{X: 2*tile + tile/2, Y: 3*tile + tile/2 + tile/4}) {
		t.Fatalf("Reach south by a quarter of a tile = %+v, want it a quarter of a tile south", got)
	}
	if got := m.Reach(from, state.West, 100*tile, 0); got != (state.Vec{X: 0, Y: 3*tile + tile/2}) {
		t.Fatalf("Reach west = %+v, want to stop at the edge of the map", got)
	}
	if got := m.Reach(from, state.North, 0, 0); got != from {
		t.Fatalf("Reach by 0 = %+v, want the same place", got)
	}
}

func TestTheSpawnIsTheMiddleOfItsTile(t *testing.T) {
	m := tilemaptest.New(t, 4, 3, tilemap.Tile{X: 1, Y: 2}, nil)
	if m.SpawnPosition() != (state.Vec{X: tile + tile/2, Y: 2*tile + tile/2}) {
		t.Fatalf("Spawn = %+v, want the middle of tile (1,2)", m.SpawnPosition())
	}
}

func TestTilesAtNegativePositionsAreOutsideTheMap(t *testing.T) {
	m := tilemaptest.New(t, 4, 3, tilemap.Tile{}, nil)
	for _, v := range []state.Vec{
		{X: -1, Y: 0}, {X: 0, Y: -1}, {X: -1000, Y: -1000}, {X: -1001, Y: 500},
	} {
		if m.Free(v) {
			t.Errorf("Free(%+v) = true, want false", v)
		}
	}
}

func TestReachDoesNotJumpOverAWallHoweverFarItGoes(t *testing.T) {
	m := tilemaptest.New(t, 10, 5, tilemap.Tile{X: 1, Y: 2}, []tilemap.Tile{{X: 4, Y: 3}})
	from := state.Vec{X: tile / 2, Y: 3*tile + tile/2}
	if got := m.Reach(
		from, state.East, 2_000_000_000, 0,
	); got != (state.Vec{X: 4*tile - 1, Y: 3*tile + tile/2}) {
		t.Fatalf("Reach east by a huge distance = %+v, want it stopped by the wall", got)
	}
	open := tilemaptest.New(t, 10, 5, tilemap.Tile{X: 1, Y: 2}, nil)
	if got := open.Reach(
		from, state.East, 2_000_000_000, 0,
	); got != (state.Vec{X: 10*tile - 1, Y: 3*tile + tile/2}) {
		t.Fatalf("Reach east by a huge distance = %+v, want the last unit of the map", got)
	}
}

func TestADiagonalMoveDoesNotSlipBetweenTwoWallsThatTouchAtACorner(t *testing.T) {
	// Walls at (4,3) and (5,2) touch at a corner; the way from tile (4,2) to tile (5,3) goes
	// between them.
	m := tilemaptest.New(
		t, 10, 5, tilemap.Tile{X: 1, Y: 2}, []tilemap.Tile{{X: 4, Y: 3}, {X: 5, Y: 2}},
	)
	from := state.Vec{X: 4*tile + tile/2, Y: 2*tile + tile/2}
	got := m.Reach(from, state.SouthEast, 2*tile, 0)
	if tileOfUnits(got) != (tilemap.Tile{X: 4, Y: 2}) {
		t.Fatalf("Reach south-east = %+v, want it to stay in tile (4,2)", got)
	}
}

func TestADiagonalMoveInTheOpenGoesTheWholeDistanceInBothAxes(t *testing.T) {
	m := tilemaptest.New(t, 10, 5, tilemap.Tile{X: 1, Y: 2}, nil)
	from := state.Vec{X: tile + tile/2, Y: tile + tile/2}
	if got := m.Reach(
		from, state.SouthEast, tile, 0,
	); got != (state.Vec{X: 2*tile + tile/2, Y: 2*tile + tile/2}) {
		t.Fatalf("Reach south-east = %+v, want one tile in each axis", got)
	}
	if got := m.Reach(
		from, state.NorthWest, tile, 0,
	); got != (state.Vec{X: tile / 2, Y: tile / 2}) {
		t.Fatalf("Reach north-west = %+v, want one tile in each axis", got)
	}
}

func TestAPositionThatCannotBeStoodOnDoesNotMove(t *testing.T) {
	m := tilemaptest.New(t, 4, 3, tilemap.Tile{X: 1, Y: 1}, []tilemap.Tile{{X: 2, Y: 1}})
	inWall := state.Vec{X: 2*tile + 5, Y: tile + 5}
	if got := m.Reach(inWall, state.West, tile, 0); got != inWall {
		t.Fatalf("Reach from a wall = %+v, want it not to move", got)
	}
	if got := m.Reach(
		state.Vec{X: -5, Y: 0}, state.East, tile, 0,
	); got != (state.Vec{X: -5, Y: 0}) {
		t.Fatalf("Reach from outside the map = %+v, want it not to move", got)
	}
}

func TestAMapThatIsNotWellFormedIsRefused(t *testing.T) {
	for name, make := range map[string]func() (*tilemap.TileMap, error){
		"no size": func() (*tilemap.TileMap, error) {
			return tilemap.New(0, 3, tilemap.Tile{}, nil)
		},
		"wall outside": func() (*tilemap.TileMap, error) {
			return tilemap.New(4, 3, tilemap.Tile{}, []tilemap.Tile{{X: 4, Y: 0}})
		},
		"spawn outside": func() (*tilemap.TileMap, error) {
			return tilemap.New(4, 3, tilemap.Tile{X: 9, Y: 9}, nil)
		},
		"spawn in a wall": func() (*tilemap.TileMap, error) {
			return tilemap.New(4, 3, tilemap.Tile{X: 1, Y: 1}, []tilemap.Tile{{X: 1, Y: 1}})
		},
		"too many tiles": func() (*tilemap.TileMap, error) {
			return tilemap.New(4097, 4096, tilemap.Tile{}, nil)
		},
		"side too long": func() (*tilemap.TileMap, error) {
			return tilemap.New(1<<20+1, 1, tilemap.Tile{}, nil)
		},
		"sides overflow": func() (*tilemap.TileMap, error) {
			return tilemap.New(1<<31-1, 1<<31-1, tilemap.Tile{}, nil)
		},
	} {
		m, err := make()
		if m != nil || !errors.Is(err, tilemap.ErrInvalid) {
			t.Errorf("%s: got map %v and error %v, want no map and ErrInvalid", name, m, err)
		}
	}
}

func TestTheLargestMapsAreAccepted(t *testing.T) {
	for _, size := range [][2]int32{{4096, 4096}, {1 << 20, 16}} {
		if m, err := tilemap.New(size[0], size[1], tilemap.Tile{}, nil); m == nil ||
			err != nil {
			t.Errorf("%d by %d: got %v, %v", size[0], size[1], m, err)
		}
	}
}

// tileOfUnits is the tile a position is in, for the assertions of the tests.
func tileOfUnits(v state.Vec) tilemap.Tile { return tilemap.Tile{X: v.X / tile, Y: v.Y / tile} }

func TestABodyWithARadiusStopsWhenItsEdgeTouchesAWall(t *testing.T) {
	m := tilemaptest.New(t, 10, 5, tilemap.Tile{X: 1, Y: 2}, []tilemap.Tile{{X: 4, Y: 3}})
	from := state.Vec{X: tile / 2, Y: 3*tile + tile/2}
	const radius = 300
	if got := m.Reach(
		from, state.East, 5*tile, radius,
	); got != (state.Vec{X: 4*tile - 1 - radius, Y: 3*tile + tile/2}) {
		t.Fatalf("Reach east = %+v, want the body's edge on the last unit before the wall", got)
	}
}

func TestABodyIsStoppedByAWallBesideItsPathThatAPointWouldPass(t *testing.T) {
	// The path is along the middle of row 2; the wall is in row 3, below it. A point passes, a body
	// that reaches 600 units to each side touches row 3 (the row is 1024 high and the middle is 512
	// from its edge).
	m := tilemaptest.New(t, 10, 5, tilemap.Tile{X: 1, Y: 2}, []tilemap.Tile{{X: 4, Y: 3}})
	from := state.Vec{X: 2 * tile, Y: 2*tile + tile/2}
	if got := m.Reach(from, state.East, 20*tile, 0); got.X != 10*tile-1 {
		t.Fatalf("a point stopped at %+v, want it to pass", got)
	}
	if got := m.Reach(from, state.East, 9*tile, 600); got.X != 4*tile-1-600 {
		t.Fatalf("a body of radius 600 stopped at %+v, want it stopped by the wall beside it", got)
	}
}

func TestABodyDoesNotFitWhereItsEdgeTouchesAWallOrTheEdgeOfTheMap(t *testing.T) {
	m := tilemaptest.New(t, 4, 3, tilemap.Tile{X: 1, Y: 1}, []tilemap.Tile{{X: 2, Y: 1}})
	for name, c := range map[string]struct {
		at     state.Vec
		radius int32
		want   bool
	}{
		"point in the middle":      {state.Vec{X: tile + tile/2, Y: tile + tile/2}, 0, true},
		"body that fits the tile":  {state.Vec{X: tile + tile/2, Y: tile + tile/2}, 511, true},
		"body touching the wall":   {state.Vec{X: tile + tile/2, Y: tile + tile/2}, 512, false},
		"body over the map's edge": {state.Vec{X: 100, Y: tile + tile/2}, 200, false},
		"body at the map's corner": {state.Vec{X: 200, Y: 200}, 200, true},
	} {
		if got := m.FreeForBody(c.at, c.radius); got != c.want {
			t.Errorf("%s: FreeForBody = %v, want %v", name, got, c.want)
		}
	}
}

func TestABodyDoesNotSlipBetweenTwoWallsThatTouchAtACorner(t *testing.T) {
	m := tilemaptest.New(
		t, 10, 5, tilemap.Tile{X: 1, Y: 2}, []tilemap.Tile{{X: 4, Y: 3}, {X: 5, Y: 2}},
	)
	from := state.Vec{X: 4*tile + tile/2, Y: 2*tile + tile/2}
	if got := m.Reach(from, state.SouthEast, 2*tile, 100); tileOfUnits(
		got,
	) != (tilemap.Tile{X: 4, Y: 2}) {
		t.Fatalf("Reach south-east = %+v, want it to stay in tile (4,2)", got)
	}
}

func TestABodyThatDoesNotFitDoesNotMove(t *testing.T) {
	m := tilemaptest.New(t, 4, 3, tilemap.Tile{X: 1, Y: 1}, []tilemap.Tile{{X: 2, Y: 1}})
	at := state.Vec{X: 2*tile - 100, Y: tile + tile/2}
	if got := m.Reach(at, state.West, tile, 200); got != at {
		t.Fatalf("Reach = %+v, want a body that touches a wall not to move", got)
	}
}

func BenchmarkReach(b *testing.B) {
	m := tilemaptest.New(b, 200, 200, tilemap.Tile{}, nil)
	for _, radius := range []int32{0, 256, 511, 2048} {
		// The first place where the body fits: the middle of the first tile, or the body's radius
		// from the corner.
		corner := max(tile/2, radius)
		from := state.Vec{X: corner, Y: corner}
		b.Run(fmt.Sprintf("radius %d/a step of a quarter tile", radius), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				m.Reach(from, state.East, tile/4, radius)
			}
		})
		b.Run(fmt.Sprintf("radius %d/diagonal, 10 tiles", radius), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				m.Reach(from, state.SouthEast, 10*tile, radius)
			}
		})
		b.Run(fmt.Sprintf("radius %d/the whole map", radius), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				m.Reach(from, state.East, 2_000_000_000, radius)
			}
		})
	}
}

// BenchmarkReachLargeMap moves from many places of a map of 4096 by 4096 tiles, so that the tiles
// looked at are not the same ones in the cache each time.
func BenchmarkReachLargeMap(b *testing.B) {
	const side = 4096
	m := tilemaptest.New(b, side, side, tilemap.Tile{}, nil)
	r := rand.New(rand.NewPCG(1, 2))
	froms := make([]state.Vec, 1<<12)
	for i := range froms {
		froms[i] = state.Vec{X: (1 + r.Int32N(side-20)) * tile, Y: (1 + r.Int32N(side-20)) * tile}
	}
	for _, radius := range []int32{256, 2048} {
		b.Run(fmt.Sprintf("radius %d/a step of a quarter tile", radius), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				m.Reach(froms[i&(len(froms)-1)], state.East, tile/4, radius)
			}
		})
		b.Run(fmt.Sprintf("radius %d/diagonal, 10 tiles", radius), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				m.Reach(froms[i&(len(froms)-1)], state.SouthEast, 10*tile, radius)
			}
		})
	}
}

func TestABodyOfManyTilesIsStoppedByAWallAndDoesNotFitInANarrowPlace(t *testing.T) {
	m := tilemaptest.New(t, 20, 10, tilemap.Tile{X: 4, Y: 4}, []tilemap.Tile{{X: 12, Y: 5}})
	const radius = 2 * tile // a boss five tiles wide
	from := state.Vec{X: 4*tile + tile/2, Y: 4*tile + tile/2}
	if got := m.Reach(
		from, state.East, 30*tile, radius,
	); got != (state.Vec{X: 12*tile - 1 - radius, Y: 4*tile + tile/2}) {
		t.Fatalf("Reach east = %+v, want the boss stopped by the wall that its edge touches", got)
	}
	if m.FreeForBody(state.Vec{X: tile / 2, Y: tile / 2}, radius) {
		t.Fatal("a boss five tiles wide fits in the corner of the map")
	}
}

// reachByEveryUnit is the answer by the plain way: one unit at a time, as long as the body fits.
func reachByEveryUnit(
	m *tilemap.TileMap, from state.Vec, dir state.Dir, distance, radius int32,
) state.Vec {
	at := from
	for range distance {
		next := state.Vec{X: at.X + dir.Vec().X, Y: at.Y + dir.Vec().Y}
		if !m.FreeForBody(next, radius) {
			break
		}
		at = next
	}
	return at
}

func TestReachIsTheSameAsGoingOneUnitAtATime(t *testing.T) {
	random := rand.New(rand.NewPCG(1, 2))
	var walls []tilemap.Tile
	for range 40 {
		w := tilemap.Tile{X: random.Int32N(20), Y: random.Int32N(20)}
		if w != (tilemap.Tile{X: 10, Y: 10}) {
			walls = append(walls, w)
		}
	}
	m := tilemaptest.New(t, 20, 20, tilemap.Tile{X: 10, Y: 10}, walls)
	dirs := []state.Dir{
		state.North,
		state.NorthEast,
		state.East,
		state.SouthEast,
		state.South,
		state.SouthWest,
		state.West,
		state.NorthWest,
	}
	checked := 0
	for range 4000 {
		radius := []int32{0, 1, 100, 511, 700, 1500, 2048}[random.IntN(7)]
		from := state.Vec{X: random.Int32N(20 * tile), Y: random.Int32N(20 * tile)}
		if !m.FreeForBody(from, radius) {
			continue
		}
		dir, distance := dirs[random.IntN(len(dirs))], random.Int32N(6*tile)
		want := reachByEveryUnit(m, from, dir, distance, radius)
		if got := m.Reach(from, dir, distance, radius); got != want {
			t.Fatalf(
				"Reach(%+v, %v, %d, radius %d) = %+v, want %+v by going one unit at a time",
				from,
				dir,
				distance,
				radius,
				got,
				want,
			)
		}
		checked++
	}
	if checked < 300 {
		t.Fatalf("only %d moves were checked", checked)
	}
}

func TestABodyOfNoSizeThatCanBeMeasuredFitsNowhere(t *testing.T) {
	m := tilemaptest.New(t, 10, 10, tilemap.Tile{X: 1, Y: 1}, nil)
	at := state.Vec{X: 5 * tile, Y: 5 * tile}
	for _, radius := range []int32{-1, -2048, math.MinInt32} {
		if m.FreeForBody(at, radius) {
			t.Fatalf("a body of radius %d fits", radius)
		}
		if got := m.Reach(at, state.East, 3*tile, radius); got != at {
			t.Fatalf("a body of radius %d moved to %+v", radius, got)
		}
	}
}

func TestAHugeRadiusOrPositionDoesNotWrapAround(t *testing.T) {
	m := tilemaptest.New(t, 10, 10, tilemap.Tile{X: 1, Y: 1}, nil)
	at := state.Vec{X: 5 * tile, Y: 5 * tile}
	for _, radius := range []int32{math.MaxInt32, math.MaxInt32 - 1000, 1 << 30} {
		if m.FreeForBody(at, radius) {
			t.Fatalf("a body of radius %d fits in a map of 10 tiles", radius)
		}
		if got := m.Reach(at, state.East, tile, radius); got != at {
			t.Fatalf("a body of radius %d moved to %+v", radius, got)
		}
	}
	far := state.Vec{X: math.MaxInt32, Y: math.MaxInt32}
	if m.FreeForBody(far, 1000) || m.Reach(far, state.East, tile, 0) != far {
		t.Fatal("a body far outside the map was taken for one inside it")
	}
}

func TestAMapComesBackAsItWasWrittenAndACloneSharesNothingWithIt(t *testing.T) {
	m := tilemaptest.New(
		t, 4, 3, tilemap.Tile{X: 1, Y: 1}, []tilemap.Tile{{X: 2, Y: 0}, {X: 0, Y: 2}},
	)
	var w binary.Writer
	m.Encode(&w)
	r := binary.NewReader(w.B)
	got, err := tilemap.Decode(r, 1)
	if err != nil || r.Left() != 0 || !reflect.DeepEqual(got, m) {
		t.Fatalf("came back as %+v, error %v, %d bytes left", got, err, r.Left())
	}
	for n := range w.B { // every proper prefix
		if _, err := tilemap.Decode(binary.NewReader(w.B[:n]), 1); err == nil {
			t.Fatalf("%d of %d bytes were read", n, len(w.B))
		}
	}
	clone := m.Clone()
	if !reflect.DeepEqual(clone, m) || clone == m {
		t.Fatal("the clone is not a copy that says the same")
	}
	for _, v := range []state.Vec{{X: 2*tile + 5, Y: 5}, {X: 5, Y: 2*tile + 5}} {
		if clone.Free(v) || m.Free(v) {
			t.Fatalf("the wall at %+v is free", v)
		}
	}
}
