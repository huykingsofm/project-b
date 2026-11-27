package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// WorldZoneLayerRegisterAction makes the world list a zonelayer: it is the message that the
// zonelayer sends its world when it starts, since the world is the one that knows which zonelayers
// there are (R28).
type WorldZoneLayerRegisterAction struct{ ZoneLayerID sim.StateID }

func (a WorldZoneLayerRegisterAction) ID() sim.ActionID { return WorldZoneLayerRegisterActionID }
func (a WorldZoneLayerRegisterAction) Priority() int16  { return PriorityResolve }
func (a WorldZoneLayerRegisterAction) Actor() any       { return nil }

// Clone returns the action itself: it holds only values.
func (a WorldZoneLayerRegisterAction) Clone() sim.Action[*worldstate.State] { return a }

// Apply lists the zonelayer.
func (a WorldZoneLayerRegisterAction) Apply(
	_ *sim.Context, s *worldstate.State, _ *sim.Registry[*worldstate.State],
	_ *sim.Plan[*worldstate.State],
) {
	s.AddZoneLayer(a.ZoneLayerID)
}

// Encode writes the ID of the zonelayer.
func (a WorldZoneLayerRegisterAction) Encode(w *binary.Writer) { w.Uvarint(uint64(a.ZoneLayerID)) }

// decodeWorldZoneLayerRegisterAction reads what Encode wrote.
func decodeWorldZoneLayerRegisterAction(
	r *binary.Reader, _ codec.Version,
) (WorldZoneLayerRegisterAction, error) {
	a := WorldZoneLayerRegisterAction{ZoneLayerID: r.Uint[sim.StateID]()}
	return a, r.Err()
}
