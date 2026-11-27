package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ZoneZoneLayerAddAction makes the zone know a zonelayer, with room for Capacity characters: it is
// the message that the zonelayer sends its zone when it starts.
type ZoneZoneLayerAddAction struct {
	ZoneLayerID sim.StateID
	Capacity    int32
}

func (a ZoneZoneLayerAddAction) ID() sim.ActionID { return ZoneZoneLayerAddActionID }
func (a ZoneZoneLayerAddAction) Priority() int16  { return PriorityResolve }
func (a ZoneZoneLayerAddAction) Actor() any       { return nil }

// Clone returns the action itself: it holds only values.
func (a ZoneZoneLayerAddAction) Clone() sim.Action[*zonestate.State] { return a }

// Apply adds the zonelayer.
func (a ZoneZoneLayerAddAction) Apply(
	_ *sim.Context, s *zonestate.State, _ *sim.Registry[*zonestate.State],
	_ *sim.Plan[*zonestate.State],
) {
	s.AddZoneLayer(a.ZoneLayerID, a.Capacity)
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a ZoneZoneLayerAddAction) Encode(w *binary.Writer) {
	w.Uvarint(uint64(a.ZoneLayerID))
	w.Varint(int64(a.Capacity))
}

// decodeZoneZoneLayerAddAction reads what Encode wrote.
func decodeZoneZoneLayerAddAction(
	r *binary.Reader, version codec.Version,
) (ZoneZoneLayerAddAction, error) {
	a := ZoneZoneLayerAddAction{ZoneLayerID: r.Uint[sim.StateID](), Capacity: r.Int[int32]()}
	return a, r.Err()
}
