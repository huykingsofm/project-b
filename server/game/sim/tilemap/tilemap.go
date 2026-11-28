// Package tilemap holds the map of a zone: its size, the tiles that cannot be walked on, and where
// characters appear. It never changes while it is played, so the state of the zone holds it and
// every copy of that state shares it. A zonelayer that wants to look at it reads it from the state
// of its zone.
package tilemap

import (
	"errors"
	"fmt"

	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/shared/bitset"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// TileSize is how many units a tile is wide and high: one of the roots of the game, which the
// server and the client both hold. It must NEVER be changed: every position, radius and distance
// is in units, and what was written (logs, checkpoints, maps) and what the client holds would mean
// something else. A tile is for whoever lays out a map.
const TileSize int32 = 1024

// The largest map: positions are int32 units, so a side of 2^20 tiles (2^30 units) fits; a map of
// 4096 by 4096 tiles takes 2 MiB of bits.
const (
	maxSide  = 1 << 20
	maxTiles = 4096 * 4096
)

// ErrInvalid is wrapped by the error of New when a map is not well formed.
var ErrInvalid = errors.New("tilemap: the map is not well formed")

// AreaOf returns the area, in units, of the tiles from min to max, both included.
func AreaOf(min, max Tile) state.Rect {
	return state.Rect{
		Min: state.Vec{X: min.X * TileSize, Y: min.Y * TileSize},
		Max: state.Vec{X: (max.X+1)*TileSize - 1, Y: (max.Y+1)*TileSize - 1},
	}
}

// Tile is a coordinate of the map: which tile, not where inside it.
type Tile struct{ X, Y int32 }

// tileOf returns the tile that the position is in.
func tileOf(v state.Vec) Tile { return tileAt(int64(v.X), int64(v.Y)) }

// tileAt returns the tile of a position given in 64 bits, so that a position plus a radius never
// wraps around.
func tileAt(x, y int64) Tile {
	return Tile{X: int32(floorDiv(x, int64(TileSize))), Y: int32(floorDiv(y, int64(TileSize)))}
}

// Center returns the position at the middle of the tile.
func (t Tile) Center() state.Vec {
	return state.Vec{X: t.X*TileSize + TileSize/2, Y: t.Y*TileSize + TileSize/2}
}

// TileMap is a map of tiles. It never changes after it is made, and every copy of a zone state
// shares it, so everything in it is private and read through methods.
type TileMap struct {
	width, height int32 // in tiles
	spawn         state.Vec
	blocked       *bitset.Array // one bit for each tile, row by row
}

// Decode reads what Encode wrote, of the data version given.
func Decode(r *binary.Reader, version codec.Version) (*TileMap, error) {
	m := &TileMap{width: r.Int[int32](), height: r.Int[int32]()}
	m.spawn = state.DecodeVec(r, version)
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, err
	}
	if n > uint64(r.Left())/8 {
		return nil, fmt.Errorf("%w: %d words of tiles", binary.ErrShort, n)
	}
	words := make([]uint64, 0, n)
	for range n {
		words = append(words, r.Fixed64())
	}
	var err error
	if m.blocked, err = bitset.FromWords(int(m.width)*int(m.height), words); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return m, r.Err()
}

// New makes a map of width by height tiles with the given spawn tile and walls. It returns an
// error that wraps ErrInvalid and says what is wrong when the map is not well formed: the size is
// at least one tile each way and no larger than the limits above, the walls and the spawn are
// inside the map, and the spawn is not a wall. A map has no exit: the world joins two zones with
// exits while the game runs.
func New(width, height int32, spawn Tile, walls []Tile) (*TileMap, error) {
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
	m := &TileMap{width: width, height: height, spawn: spawn.Center(), blocked: blocked}
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
	return m, nil
}

func (m *TileMap) SpawnPosition() state.Vec { return m.spawn }

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

// Holds reports whether the area is not empty (its Min is not after its Max) and is in the map: an
// exit is an area of the map.
func (m *TileMap) Holds(area state.Rect) bool {
	return area.Min.X <= area.Max.X && area.Min.Y <= area.Max.Y &&
		area.Min.X >= 0 && area.Min.Y >= 0 &&
		int64(area.Max.X) < int64(m.width)*int64(TileSize) &&
		int64(area.Max.Y) < int64(m.height)*int64(TileSize)
}

// Encode writes the map: its size, its spawn and the tiles that cannot be walked on.
func (m *TileMap) Encode(w *binary.Writer) {
	w.Varint(int64(m.width))
	w.Varint(int64(m.height))
	m.spawn.Encode(w)
	words := m.blocked.Words()
	w.Uvarint(uint64(len(words)))
	for _, word := range words {
		w.Fixed64(word)
	}
}

// Clone returns a copy that shares nothing with the map. The map never changes, so the engines
// share one; a command or an action that holds a map is copied with its Clone like anything else
// that holds a reference (R13).
func (m *TileMap) Clone() *TileMap {
	return &TileMap{
		width: m.width, height: m.height, spawn: m.spawn, blocked: m.blocked.Clone(),
	}
}

func (m *TileMap) inside(t Tile) bool {
	return t.X >= 0 && t.Y >= 0 && t.X < m.width && t.Y < m.height
}

func (m *TileMap) isWall(t Tile) bool { return m.blocked.Get(int(t.Y)*int(m.width) + int(t.X)) }

// free reports whether a character can stand on the tile: it is in the map and not a wall.
func (m *TileMap) free(t Tile) bool { return m.inside(t) && !m.isWall(t) }

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

var _ ReadOnlyTileMap = (*TileMap)(nil)

// SpawnPosition is where characters appear: the middle of the spawn tile, which is never a wall.
// ReadOnlyTileMap is the read methods of a TileMap, which is what an action may use of the map of a
// zone it reads from the stable view. The map never changes after it is made, and this says so.
type ReadOnlyTileMap interface {
	SpawnPosition() state.Vec
	Free(v state.Vec) bool
	FreeForBody(position state.Vec, radius int32) bool
	Reach(from state.Vec, dir state.Dir, distance int32, radius int32) state.Vec
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
	tile := floorDiv(position, int64(TileSize))
	if sign > 0 {
		return (tile+1)*int64(TileSize) - position
	}
	return position - tile*int64(TileSize) + 1
}
