package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ZoneAnnounceToWorldAction tells the world of the zone that the zone is there. It is set when the
// zone is made, for its first tick: only the zone knows its own ID by then (Context.Self).
type ZoneAnnounceToWorldAction struct{}

func (a ZoneAnnounceToWorldAction) ID() sim.ActionID { return ZoneAnnounceToWorldActionID }
func (a ZoneAnnounceToWorldAction) Priority() int16  { return PriorityResolve }
func (a ZoneAnnounceToWorldAction) Actor() any       { return nil }

// Clone returns the action itself: it holds nothing.
func (a ZoneAnnounceToWorldAction) Clone() sim.Action[*zonestate.State] { return a }

// Apply gives the world the zone.
func (a ZoneAnnounceToWorldAction) Apply(
	c *sim.Context, s *zonestate.State, r *sim.Registry[*zonestate.State],
	_ *sim.Plan[*zonestate.State],
) {
	r.Send(s.WorldID(), onfail.Retry{}, WorldZoneRegisterAction{ZoneID: c.Self})
}

// Encode writes nothing: the action holds no field.
func (a ZoneAnnounceToWorldAction) Encode(*binary.Writer) {}

// decodeZoneAnnounceToWorldAction reads what Encode wrote.
func decodeZoneAnnounceToWorldAction(
	*binary.Reader, codec.Version,
) (ZoneAnnounceToWorldAction, error) {
	return ZoneAnnounceToWorldAction{}, nil
}
