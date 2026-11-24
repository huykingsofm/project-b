package state

import (
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// Vec is a position or a distance, in units. Every position, radius and distance of the simulation
// is in units; only the map knows how it is cut into tiles (package tilemap).
type Vec struct{ X, Y int32 }

// Add returns the sum of the two vectors.
func (v Vec) Add(o Vec) Vec { return Vec{v.X + o.X, v.Y + o.Y} }

// Scaled returns the vector multiplied by n.
func (v Vec) Scaled(n int32) Vec { return Vec{v.X * n, v.Y * n} }

// Encode writes a vector: its two coordinates.
func (v Vec) Encode(w *binary.Writer) {
	w.Varint(int64(v.X))
	w.Varint(int64(v.Y))
}

// Dir is a direction of movement.
type Dir uint8

// Vec returns the step of one unit in the direction, in each axis it moves in.
func (d Dir) Vec() Vec {
	if int(d) >= len(dirVec) {
		return Vec{}
	}
	return dirVec[d]
}

// The eight directions, and none.
const (
	NoDir Dir = iota
	North
	NorthEast
	East
	SouthEast
	South
	SouthWest
	West
	NorthWest
)

var dirVec = [...]Vec{
	NoDir: {0, 0}, North: {0, -1}, NorthEast: {1, -1}, East: {1, 0}, SouthEast: {1, 1},
	South: {0, 1}, SouthWest: {-1, 1}, West: {-1, 0}, NorthWest: {-1, -1},
}

// Rect is a rectangle of units, both ends included.
type Rect struct{ Min, Max Vec }

// Grown returns the rectangle with n more units on every side.
func (r Rect) Grown(n int32) Rect {
	return Rect{Min: Vec{X: r.Min.X - n, Y: r.Min.Y - n}, Max: Vec{X: r.Max.X + n, Y: r.Max.Y + n}}
}

// Intersects reports whether the rectangles share at least one position.
func (r Rect) Intersects(o Rect) bool {
	return r.Min.X <= o.Max.X && o.Min.X <= r.Max.X && r.Min.Y <= o.Max.Y && o.Min.Y <= r.Max.Y
}

// Contains reports whether the position is inside the rectangle.
func (r Rect) Contains(v Vec) bool {
	return v.X >= r.Min.X && v.X <= r.Max.X && v.Y >= r.Min.Y && v.Y <= r.Max.Y
}

// RectCovering returns the smallest rectangle that holds both positions.
func RectCovering(a, b Vec) Rect {
	return Rect{
		Min: Vec{X: min(a.X, b.X), Y: min(a.Y, b.Y)}, Max: Vec{X: max(a.X, b.X), Y: max(a.Y, b.Y)},
	}
}

// DecodeVec reads what Vec.Encode wrote, of the data version given. A reader that has failed gives
// the zero vector.
func DecodeVec(r *binary.Reader, version codec.Version) Vec {
	return Vec{X: r.Int[int32](), Y: r.Int[int32]()}
}
