package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// WorldZoneRegisterAction makes the world list a zone: it is the message that the zone sends its
// world when it starts, since the world is the one that knows which zones there are (R28).
type WorldZoneRegisterAction struct{ ZoneID sim.StateID }

func (a WorldZoneRegisterAction) ID() sim.ActionID { return WorldZoneRegisterActionID }
func (a WorldZoneRegisterAction) Priority() int16  { return PriorityResolve }
func (a WorldZoneRegisterAction) Actor() any       { return nil }

// Clone returns the action itself: it holds only values.
func (a WorldZoneRegisterAction) Clone() sim.Action[*worldstate.State] { return a }

// Apply lists the zone.
func (a WorldZoneRegisterAction) Apply(
	_ *sim.Context, s *worldstate.State, _ *sim.Registry[*worldstate.State],
	_ *sim.Plan[*worldstate.State],
) {
	s.AddZone(a.ZoneID)
}

// Encode writes the ID of the zone.
func (a WorldZoneRegisterAction) Encode(w *binary.Writer) { w.Uvarint(uint64(a.ZoneID)) }

// decodeWorldZoneRegisterAction reads what Encode wrote.
func decodeWorldZoneRegisterAction(
	r *binary.Reader, _ codec.Version,
) (WorldZoneRegisterAction, error) {
	a := WorldZoneRegisterAction{ZoneID: r.Uint[sim.StateID]()}
	return a, r.Err()
}
