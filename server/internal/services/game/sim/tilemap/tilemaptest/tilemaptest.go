// Package tilemaptest makes maps for tests.
package tilemaptest

import (
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/tilemap"
)

// New is tilemap.New that fails the test when the map is not well formed.
func New(
	t testing.TB,
	width,
	height int32,
	spawn tilemap.Tile,
	walls []tilemap.Tile,
	exits []tilemap.ExitDefinition,
) *tilemap.TileMap {
	t.Helper()
	m, err := tilemap.New(width, height, spawn, walls, exits)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
