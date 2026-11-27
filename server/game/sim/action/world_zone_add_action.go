package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// WorldZoneAddAction asks the coordinator for a zone of the world, on a map. The world lists the
// zone when the zone tells it that it is there (ZoneAnnounceToWorldAction), not here (R28): the
// request only makes the engine.
type WorldZoneAddAction struct {
	Tiles *tilemap.TileMap
}

func (a WorldZoneAddAction) ID() sim.ActionID { return WorldZoneAddActionID }
func (a WorldZoneAddAction) Priority() int16  { return PriorityResolve }
func (a WorldZoneAddAction) Actor() any       { return nil }

// Clone returns a copy that shares no map with the action.
func (a WorldZoneAddAction) Clone() sim.Action[*worldstate.State] {
	return WorldZoneAddAction{Tiles: a.Tiles.Clone()}
}

// Apply asks for the zone, which is of this world.
func (a WorldZoneAddAction) Apply(
	c *sim.Context, _ *worldstate.State, r *sim.Registry[*worldstate.State],
	_ *sim.Plan[*worldstate.State],
) {
	r.RequestCreateEngine(
		onfail.Retry{}, zonestate.NewState(c.Self, a.Tiles), ZoneAnnounceToWorldAction{},
	)
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a WorldZoneAddAction) Encode(w *binary.Writer) { a.Tiles.Encode(w) }

// decodeWorldZoneAddAction reads what Encode wrote.
func decodeWorldZoneAddAction(
	r *binary.Reader, version codec.Version,
) (WorldZoneAddAction, error) {
	tiles, err := tilemap.Decode(r, version)
	return WorldZoneAddAction{Tiles: tiles}, err
}
