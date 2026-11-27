package command

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// WorldZoneAddCommand asks the world to have a zone, on a map, made.
type WorldZoneAddCommand struct {
	Tiles *tilemap.TileMap
}

func (WorldZoneAddCommand) ID() sim.CommandID { return WorldZoneAddCommandID }

// Clone returns a copy that shares no map with the command.
func (c WorldZoneAddCommand) Clone() sim.Command[*worldstate.State] {
	return WorldZoneAddCommand{Tiles: c.Tiles.Clone()}
}

func (c WorldZoneAddCommand) Translate() []sim.Action[*worldstate.State] {
	return []sim.Action[*worldstate.State]{action.WorldZoneAddAction{Tiles: c.Tiles}}
}

// Encode writes the fields of the command, not its ID: sim.EncodeCommand writes the ID first.
func (c WorldZoneAddCommand) Encode(w *binary.Writer) error {
	c.Tiles.Encode(w)
	return nil
}

// decodeWorldZoneAddCommand reads what Encode wrote.
func decodeWorldZoneAddCommand(
	r *binary.Reader, version codec.Version,
) (WorldZoneAddCommand, error) {
	tiles, err := tilemap.Decode(r, version)
	return WorldZoneAddCommand{Tiles: tiles}, err
}
