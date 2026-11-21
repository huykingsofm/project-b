package state

// Vec is a position or a distance, in tiles.
type Vec struct{ X, Y int32 }

// Add returns the sum of the two vectors.
func (v Vec) Add(o Vec) Vec { return Vec{v.X + o.X, v.Y + o.Y} }

// Dir is a direction of movement.
type Dir uint8

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

var dirVec = [...]Vec{NoDir: {0, 0}, North: {0, -1}, NorthEast: {1, -1}, East: {1, 0}, SouthEast: {1, 1},
	South: {0, 1}, SouthWest: {-1, 1}, West: {-1, 0}, NorthWest: {-1, -1}}

// Vec returns the step of one tile in the direction.
func (d Dir) Vec() Vec {
	if int(d) >= len(dirVec) {
		return Vec{}
	}
	return dirVec[d]
}
