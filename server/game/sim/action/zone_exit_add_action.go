package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ZoneExitAddAction puts an exit in the zone: the area (in units) that leads to the zone ToZoneID,
// where a character comes out at Arrival. It is the message that the world sends the zone
// (WorldZoneExitAddAction).
type ZoneExitAddAction struct {
	Area     state.Rect
	ToZoneID sim.StateID
	Arrival  state.Vec
}

// decodeZoneExitAddAction reads what Encode wrote.
func decodeZoneExitAddAction(
	r *binary.Reader, version codec.Version,
) (ZoneExitAddAction, error) {
	a := ZoneExitAddAction{}
	a.Area.Min = state.DecodeVec(r, version)
	a.Area.Max = state.DecodeVec(r, version)
	a.ToZoneID = r.Uint[sim.StateID]()
	a.Arrival = state.DecodeVec(r, version)
	return a, r.Err()
}

func (a ZoneExitAddAction) ID() sim.ActionID { return ZoneExitAddActionID }

func (a ZoneExitAddAction) Priority() int16 { return PriorityResolve }

func (a ZoneExitAddAction) Actor() any { return nil }

// Clone returns the action itself: it holds only values.
func (a ZoneExitAddAction) Clone() sim.Action[*zonestate.State] { return a }

// Apply adds the exit; an area that is not in the map of the zone adds none.
func (a ZoneExitAddAction) Apply(
	_ *sim.Context, s *zonestate.State, _ *sim.ActionRegistry[*zonestate.State],
	_ *sim.Plan[*zonestate.State],
) {
	s.AddExit(a.Area, a.ToZoneID, a.Arrival)
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a ZoneExitAddAction) Encode(w *binary.Writer) {
	a.Area.Min.Encode(w)
	a.Area.Max.Encode(w)
	w.Uvarint(uint64(a.ToZoneID))
	a.Arrival.Encode(w)
}
