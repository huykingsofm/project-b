package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ZoneLayerAnnounceToZoneAction tells the zone of the zonelayer that the zonelayer is there and has
// room for Capacity characters. It is set when the zonelayer is made, for its first tick: only the
// zonelayer knows its own ID by then (Context.Self).
type ZoneLayerAnnounceToZoneAction struct {
	Capacity int32
}

func (a ZoneLayerAnnounceToZoneAction) ID() sim.ActionID { return ZoneLayerAnnounceToZoneActionID }
func (a ZoneLayerAnnounceToZoneAction) Priority() int16  { return PriorityResolve }
func (a ZoneLayerAnnounceToZoneAction) Actor() any       { return nil }

// Clone returns the action itself: it holds only values.
func (a ZoneLayerAnnounceToZoneAction) Clone() sim.Action[*zonelayerstate.State] { return a }

// Apply gives the zone the zonelayer.
func (a ZoneLayerAnnounceToZoneAction) Apply(
	c *sim.Context, s *zonelayerstate.State, r *sim.Registry[*zonelayerstate.State],
	_ *sim.Plan[*zonelayerstate.State],
) {
	r.Send(
		s.ZoneID(), onfail.Drop{},
		ZoneZoneLayerAddAction{ZoneLayerID: c.Self, Capacity: a.Capacity},
	)
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a ZoneLayerAnnounceToZoneAction) Encode(w *binary.Writer) { w.Varint(int64(a.Capacity)) }

// decodeZoneLayerAnnounceToZoneAction reads what Encode wrote.
func decodeZoneLayerAnnounceToZoneAction(
	r *binary.Reader, version codec.Version,
) (ZoneLayerAnnounceToZoneAction, error) {
	a := ZoneLayerAnnounceToZoneAction{Capacity: r.Int[int32]()}
	return a, r.Err()
}
