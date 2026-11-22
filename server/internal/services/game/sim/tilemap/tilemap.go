// Package tilemap holds the map of a zone: its size, the tiles that cannot be walked on, where
// characters appear, and the exits (as entities). It never changes while it is played, so the state
// of the zone holds it and every copy of that state shares it. A zonelayer that wants to look at it
// reads it from the state of its zone.
package tilemap

import (
	"errors"
	"fmt"
	"slices"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state/entity"
	"github.com/huykingsofm/project-b/server/internal/shared/bitset"
)

// tileSize is how many units a tile is wide and high. It is a detail of the map: nothing outside
// this package knows it, and everything outside sees positions in units.
const tileSize int32 = 1024

// Tile is a coordinate of the map: which tile, not where inside it. Only the constructor of a map
// takes tiles.
type Tile struct{ X, Y int32 }

// center returns the position at the middle of the tile.
func (t Tile) center() state.Vec {
	return state.Vec{X: t.X*tileSize + tileSize/2, Y: t.Y*tileSize + tileSize/2}
}

// tileOf returns the tile that the position is in.
func tileOf(v state.Vec) Tile { return tileAt(int64(v.X), int64(v.Y)) }

// tileAt returns the tile of a position given in 64 bits, so that a position plus a radius never
// wraps around.
func tileAt(x, y int64) Tile {
	return Tile{X: int32(floorDiv(x, int64(tileSize))), Y: int32(floorDiv(y, int64(tileSize)))}
}

// floorDiv divides and rounds down, so that the tiles at negative positions are numbered the same
// way.
func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// ExitDefinition describes an exit of a map in tiles: the tiles from Min to Max, both included,
// lead to the zone ToZoneID, where a character comes out in the middle of the tile Arrival. The map
// turns it into an entity in units.
type ExitDefinition struct {
	Min, Max Tile
	ToZoneID state.ZoneID
	Arrival  Tile
}

// TileMap is a map of tiles. It never changes after it is made, and every copy of a zone state
// shares it, so everything in it is private and read through methods.
type TileMap struct {
	width, height int32 // in tiles
	spawn         state.Vec
	blocked       *bitset.Array // one bit for each tile, row by row
	exits         []entity.ExitEntity
}

// The largest map: positions are int32 units, so a side of 2^20 tiles (2^30 units) fits; a map of
// 4096 by 4096 tiles takes 2 MiB of bits.
const (
	maxSide  = 1 << 20
	maxTiles = 4096 * 4096
)

// ErrInvalid is wrapped by the error of New when a map is not well formed.
var ErrInvalid = errors.New("tilemap: the map is not well formed")

// New makes a map of width by height tiles with the given spawn tile, walls and exits. It returns
// an error that wraps ErrInvalid and says what is wrong when the map is not well formed: the size
// is at least one tile each way and no larger than the limits above, the walls, the spawn and the
// exits are inside the map, the spawn is not a wall, and an exit has its Min before its Max.
func New(width, height int32, spawn Tile, walls []Tile, exits []ExitDefinition) (*TileMap, error) {
	if width < 1 || height < 1 {
		return nil, fmt.Errorf(
			"%w: a map is at least one tile each way, got %d by %d", ErrInvalid, width, height,
		)
	}
	if width > maxSide || height > maxSide || int64(width)*int64(height) > maxTiles {
		return nil, fmt.Errorf(
			"%w: a map is at most %d tiles on a side and %d tiles in all, got %d by %d",
			ErrInvalid,
			maxSide,
			maxTiles,
			width,
			height,
		)
	}
	blocked, err := bitset.New(int(width) * int(height))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	m := &TileMap{width: width, height: height, spawn: spawn.center(), blocked: blocked}
	for _, t := range walls {
		if !m.inside(t) {
			return nil, fmt.Errorf(
				"%w: the wall %+v is outside the map of %d by %d", ErrInvalid, t, width, height,
			)
		}
		m.blocked.Set(int(t.Y)*int(width) + int(t.X))
	}
	if !m.inside(spawn) || m.isWall(spawn) {
		return nil, fmt.Errorf("%w: the spawn %+v is outside the map or a wall", ErrInvalid, spawn)
	}
	for _, d := range exits {
		if !m.inside(d.Min) || !m.inside(d.Max) || d.Min.X > d.Max.X || d.Min.Y > d.Max.Y {
			return nil, fmt.Errorf(
				"%w: the exit from %+v to %+v is outside the map or has its Min after its Max",
				ErrInvalid,
				d.Min,
				d.Max,
			)
		}
		m.exits = append(m.exits, entity.ExitEntity{
			Area: state.Rect{
				Min: state.Vec{X: d.Min.X * tileSize, Y: d.Min.Y * tileSize},
				Max: state.Vec{X: (d.Max.X+1)*tileSize - 1, Y: (d.Max.Y+1)*tileSize - 1},
			},
			ToZoneID: d.ToZoneID,
			Arrival:  d.Arrival.center(),
		})
	}
	return m, nil
}

// SpawnPosition is where characters appear: the middle of the spawn tile, which is never a wall.
func (m *TileMap) SpawnPosition() state.Vec { return m.spawn }

func (m *TileMap) inside(t Tile) bool {
	return t.X >= 0 && t.Y >= 0 && t.X < m.width && t.Y < m.height
}

func (m *TileMap) isWall(t Tile) bool { return m.blocked.Get(int(t.Y)*int(m.width) + int(t.X)) }

// free reports whether a character can stand on the tile: it is in the map and not a wall.
func (m *TileMap) free(t Tile) bool { return m.inside(t) && !m.isWall(t) }

// Free reports whether a point can be stood on.
func (m *TileMap) Free(v state.Vec) bool { return m.free(tileOf(v)) }

// FreeForBody reports whether a body of the radius, a square that reaches radius units from its
// position on every side (the bounding box of an entity, of any size, a boss may cover many tiles),
// fits at the position: every tile it touches can be stood on. A radius of 0 is a point, and a
// radius below 0 fits nowhere.
func (m *TileMap) FreeForBody(position state.Vec, radius int32) bool {
	if radius < 0 {
		return false
	}
	x, y, r := int64(position.X), int64(position.Y), int64(radius)
	return m.freeTiles(tileAt(x-r, y-r), tileAt(x+r, y+r))
}

// freeTiles reports whether every tile from min to max, both included, can be stood on.
func (m *TileMap) freeTiles(min, max Tile) bool {
	// A rectangle that is not inside the map has a tile outside it; and one inside is no bigger
	// than the map.
	if !m.inside(min) || !m.inside(max) {
		return false
	}
	run := int(max.X-min.X) + 1
	// A row of tiles is bits next to each other, which AnyInRange looks at in a read or two; for a
	// run of a few tiles, and for a column (its tiles are a width apart), a read of each tile costs
	// less.
	if run < 4 {
		for y := min.Y; y <= max.Y; y++ {
			for x := min.X; x <= max.X; x++ {
				if m.isWall(Tile{X: x, Y: y}) {
					return false
				}
			}
		}
		return true
	}
	for y := min.Y; y <= max.Y; y++ {
		if m.blocked.AnyInRange(int(y)*int(m.width)+int(min.X), run) {
			return false
		}
	}
	return true
}

// Reach returns the farthest position that a body of the radius can reach from a position by going
// up to distance units in a direction, and stops at the last unit before the body would touch a
// tile that cannot be stood on. It follows the whole path, and looks again each time the front of
// the body enters a new row or column of tiles, so nothing is jumped over, however far the
// distance. A body that crosses a corner diagonally needs the two tiles beside the corner free, so
// it does not slip between two walls. It never goes more than the size of the map, because the edge
// of the map stops it, and it looks at the new tiles in front of the body once for each row or
// column it enters, as many tiles as the body is wide. A body that does not fit where it is does
// not move.
func (m *TileMap) Reach(from state.Vec, dir state.Dir, distance int32, radius int32) state.Vec {
	step := dir.Vec()
	if !m.FreeForBody(from, radius) || distance <= 0 || step == (state.Vec{}) {
		return from
	}
	x, y, remaining := int64(from.X), int64(from.Y), int64(distance)
	r := int64(radius)
	for remaining > 0 {
		// How many units until the front of the body, on each axis, is in the next row or column of
		// tiles; an axis that does not move never gets there.
		toX, toY := remaining, remaining
		if step.X != 0 {
			toX = unitsToBoundary(x+int64(step.X)*r, int64(step.X))
		}
		if step.Y != 0 {
			toY = unitsToBoundary(y+int64(step.Y)*r, int64(step.Y))
		}
		units := min(remaining, toX, toY)
		nextX, nextY := x+int64(step.X)*units, y+int64(step.Y)*units
		if !m.canMove(
			state.Vec{X: int32(x), Y: int32(y)},
			state.Vec{X: int32(nextX), Y: int32(nextY)},
			radius,
		) {
			return state.Vec{
				X: int32(x + int64(step.X)*(units-1)), Y: int32(y + int64(step.Y)*(units-1)),
			}
		}
		x, y, remaining = nextX, nextY, remaining-units
	}
	return state.Vec{X: int32(x), Y: int32(y)}
}

// canMove reports whether a body of the radius, at the first position, can be at the second, which
// is no farther than the next row or column of tiles on each axis. The tiles it touched before are
// free, so it looks only at the new ones: the column and the row that its front enters, as long as
// the body is wide. When it enters a tile in a new row and a new column at once, the two tiles
// beside that corner must be free as well, so a body of any size does not slip between two walls.
func (m *TileMap) canMove(from, to state.Vec, radius int32) bool {
	dx, dy := int64(signOf(to.X-from.X)), int64(signOf(to.Y-from.Y))
	r := int64(radius)
	lo := tileAt(int64(to.X)-r, int64(to.Y)-r)
	hi := tileAt(int64(to.X)+r, int64(to.Y)+r)
	before := tileAt(int64(from.X)+dx*r, int64(from.Y)+dy*r)
	after := tileAt(int64(to.X)+dx*r, int64(to.Y)+dy*r)
	if before.X != after.X && !m.freeTiles(Tile{X: after.X, Y: lo.Y}, Tile{X: after.X, Y: hi.Y}) {
		return false
	}
	if before.Y != after.Y && !m.freeTiles(Tile{X: lo.X, Y: after.Y}, Tile{X: hi.X, Y: after.Y}) {
		return false
	}
	if before.X != after.X && before.Y != after.Y {
		return m.free(Tile{X: after.X, Y: before.Y}) && m.free(Tile{X: before.X, Y: after.Y})
	}
	return true
}

// signOf is -1, 0 or 1 for the sign of n.
func signOf(n int32) int32 {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	}
	return 0
}

// unitsToBoundary is how many units a position moves along one axis, in the direction of the sign
// (1 or -1), to be in the next tile: where the next tile begins when going up, and its last unit
// when going down.
func unitsToBoundary(position, sign int64) int64 {
	tile := floorDiv(position, int64(tileSize))
	if sign > 0 {
		return (tile+1)*int64(tileSize) - position
	}
	return position - tile*int64(tileSize) + 1
}

// Exits returns the exits of the map, as the entities a zone is given. Their IDs are not set: the
// zone gives each its own.
func (m *TileMap) Exits() []entity.ExitEntity { return slices.Clone(m.exits) }
