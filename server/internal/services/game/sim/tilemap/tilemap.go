// Package tilemap holds the map of a zone: its size, the tiles that cannot be walked on, where
// characters appear, and the exits. It never changes while it is played, so the state of the zone
// holds it and every copy of that state shares it. A zonelayer that wants to look at it reads it
// from the state of its zone.
package tilemap

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// Exit is a rectangle of a map that leads to a place in another zone.
type Exit struct {
	ID       state.ExitID
	Min, Max state.Vec // the tiles of the rectangle, both ends included
	ToZoneID state.ZoneID
	Arrival  state.Vec // where a character comes out in the other zone
}

// Contains reports whether the position is inside the exit.
func (e Exit) Contains(v state.Vec) bool {
	return v.X >= e.Min.X && v.X <= e.Max.X && v.Y >= e.Min.Y && v.Y <= e.Max.Y
}

// TileMap is a map of tiles.
type TileMap struct {
	W, H    int32
	Spawn   state.Vec
	blocked []bool
	exits   []Exit
}

// New makes a map of w by h tiles with the given walls and exits.
func New(w, h int32, spawn state.Vec, walls []state.Vec, exits []Exit) *TileMap {
	m := &TileMap{W: w, H: h, Spawn: spawn, blocked: make([]bool, int(w)*int(h)), exits: exits}
	for _, v := range walls {
		if m.inside(v) {
			m.blocked[int(v.Y)*int(w)+int(v.X)] = true
		}
	}
	return m
}

func (m *TileMap) inside(v state.Vec) bool { return v.X >= 0 && v.Y >= 0 && v.X < m.W && v.Y < m.H }

// Free reports whether a character can stand on the tile.
func (m *TileMap) Free(v state.Vec) bool {
	return m.inside(v) && !m.blocked[int(v.Y)*int(m.W)+int(v.X)]
}

// ExitAt returns the exit that the position is in.
func (m *TileMap) ExitAt(v state.Vec) (Exit, bool) {
	for _, e := range m.exits {
		if e.Contains(v) {
			return e, true
		}
	}
	return Exit{}, false
}

// ExitByID returns the exit with the ID.
func (m *TileMap) ExitByID(exitID state.ExitID) (Exit, bool) {
	for _, e := range m.exits {
		if e.ID == exitID {
			return e, true
		}
	}
	return Exit{}, false
}
