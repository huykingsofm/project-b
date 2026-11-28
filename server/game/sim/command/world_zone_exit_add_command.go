package command

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// WorldZoneExitAddCommand asks the world to put an exit in a zone that leads to another: the area
// (in units) of FromZoneID that leads to ToZoneID, where a character comes out at Arrival.
type WorldZoneExitAddCommand struct {
	FromZoneID sim.StateID
	Area       state.Rect
	ToZoneID   sim.StateID
	Arrival    state.Vec
}

// decodeWorldZoneExitAddCommand reads what Encode wrote.
func decodeWorldZoneExitAddCommand(
	r *binary.Reader, version codec.Version,
) (WorldZoneExitAddCommand, error) {
	c := WorldZoneExitAddCommand{FromZoneID: r.Uint[sim.StateID]()}
	c.Area.Min = state.DecodeVec(r, version)
	c.Area.Max = state.DecodeVec(r, version)
	c.ToZoneID = r.Uint[sim.StateID]()
	c.Arrival = state.DecodeVec(r, version)
	return c, r.Err()
}

func (WorldZoneExitAddCommand) ID() sim.CommandID { return WorldZoneExitAddCommandID }

// Clone returns the command itself: it holds only values.
func (c WorldZoneExitAddCommand) Clone() sim.Command[*worldstate.State] { return c }

func (c WorldZoneExitAddCommand) Translate() []sim.Action[*worldstate.State] {
	return []sim.Action[*worldstate.State]{action.WorldZoneExitAddAction(c)}
}

// Encode writes the fields of the command, not its ID: sim.EncodeCommand writes the ID first.
func (c WorldZoneExitAddCommand) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(c.FromZoneID))
	c.Area.Min.Encode(w)
	c.Area.Max.Encode(w)
	w.Uvarint(uint64(c.ToZoneID))
	c.Arrival.Encode(w)
	return nil
}
