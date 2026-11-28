package action

import (
	"slices"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// WorldZoneExitAddAction joins a zone to another: an exit in the zone FromZoneID, on the area (in
// units) that leads to the zone ToZoneID, where a character comes out at Arrival. The world knows
// the zones, so it is the one that joins them: it asks for the exit only when it lists both. To
// join two zones both ways it is given twice.
type WorldZoneExitAddAction struct {
	FromZoneID sim.StateID
	Area       state.Rect
	ToZoneID   sim.StateID
	Arrival    state.Vec
}

// decodeWorldZoneExitAddAction reads what Encode wrote.
func decodeWorldZoneExitAddAction(
	r *binary.Reader, version codec.Version,
) (WorldZoneExitAddAction, error) {
	a := WorldZoneExitAddAction{FromZoneID: r.Uint[sim.StateID]()}
	a.Area.Min = state.DecodeVec(r, version)
	a.Area.Max = state.DecodeVec(r, version)
	a.ToZoneID = r.Uint[sim.StateID]()
	a.Arrival = state.DecodeVec(r, version)
	return a, r.Err()
}

func (a WorldZoneExitAddAction) ID() sim.ActionID { return WorldZoneExitAddActionID }

func (a WorldZoneExitAddAction) Priority() int16 { return PriorityResolve }

func (a WorldZoneExitAddAction) Actor() any { return nil }

// Clone returns the action itself: it holds only values.
func (a WorldZoneExitAddAction) Clone() sim.Action[*worldstate.State] { return a }

// Apply sends the exit to the zone it stands in.
func (a WorldZoneExitAddAction) Apply(
	_ *sim.Context, s *worldstate.State, r *sim.ActionRegistry[*worldstate.State],
	_ *sim.Plan[*worldstate.State],
) {
	zones := s.ZoneIDs()
	if !slices.Contains(zones, a.FromZoneID) || !slices.Contains(zones, a.ToZoneID) {
		return
	}
	r.Send(
		a.FromZoneID, onfail.Drop{},
		ZoneExitAddAction{Area: a.Area, ToZoneID: a.ToZoneID, Arrival: a.Arrival},
	)
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a WorldZoneExitAddAction) Encode(w *binary.Writer) {
	w.Uvarint(uint64(a.FromZoneID))
	a.Area.Min.Encode(w)
	a.Area.Max.Encode(w)
	w.Uvarint(uint64(a.ToZoneID))
	a.Arrival.Encode(w)
}
