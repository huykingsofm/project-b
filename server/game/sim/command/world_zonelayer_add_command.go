package command

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// WorldZoneLayerAddCommand asks the world to have a zonelayer of a zone made.
type WorldZoneLayerAddCommand struct {
	ZoneID     sim.StateID
	Capacity   int32
	RegenEvery uint64
}

// decodeWorldZoneLayerAddCommand reads what Encode wrote.
func decodeWorldZoneLayerAddCommand(
	r *binary.Reader, version codec.Version,
) (WorldZoneLayerAddCommand, error) {
	c := WorldZoneLayerAddCommand{
		ZoneID: r.Uint[sim.StateID](), Capacity: r.Int[int32](), RegenEvery: r.Uvarint(),
	}
	return c, r.Err()
}

func (WorldZoneLayerAddCommand) ID() sim.CommandID { return WorldZoneLayerAddCommandID }

// Clone returns the command itself: it holds only values.
func (c WorldZoneLayerAddCommand) Clone() sim.Command[*worldstate.State] { return c }

func (c WorldZoneLayerAddCommand) Translate() []sim.Action[*worldstate.State] {
	return []sim.Action[*worldstate.State]{
		action.WorldZoneLayerAddAction{
			ZoneID: c.ZoneID, Capacity: c.Capacity, RegenEvery: c.RegenEvery,
		},
	}
}

// Encode writes the fields of the command, not its ID: sim.EncodeCommand writes the ID first.
func (c WorldZoneLayerAddCommand) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(c.ZoneID))
	w.Varint(int64(c.Capacity))
	w.Uvarint(c.RegenEvery)
	return nil
}
