package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ZoneLayerAnnounceToWorldAction tells the world of the zonelayer that the zonelayer is there. It
// is set when the zonelayer is made, for its first tick: only the zonelayer knows its own ID by
// then (Context.Self).
type ZoneLayerAnnounceToWorldAction struct{}

func (a ZoneLayerAnnounceToWorldAction) ID() sim.ActionID {
	return ZoneLayerAnnounceToWorldActionID
}
func (a ZoneLayerAnnounceToWorldAction) Priority() int16 { return PriorityResolve }
func (a ZoneLayerAnnounceToWorldAction) Actor() any      { return nil }

// Clone returns the action itself: it holds nothing.
func (a ZoneLayerAnnounceToWorldAction) Clone() sim.Action[*zonelayerstate.State] { return a }

// Apply gives the world the zonelayer.
func (a ZoneLayerAnnounceToWorldAction) Apply(
	c *sim.Context, s *zonelayerstate.State, r *sim.Registry[*zonelayerstate.State],
	_ *sim.Plan[*zonelayerstate.State],
) {
	r.Send(s.WorldID(), onfail.Retry{}, WorldZoneLayerRegisterAction{ZoneLayerID: c.Self})
}

// Encode writes nothing: the action holds no field.
func (a ZoneLayerAnnounceToWorldAction) Encode(*binary.Writer) {}

// decodeZoneLayerAnnounceToWorldAction reads what Encode wrote.
func decodeZoneLayerAnnounceToWorldAction(
	*binary.Reader, codec.Version,
) (ZoneLayerAnnounceToWorldAction, error) {
	return ZoneLayerAnnounceToWorldAction{}, nil
}
