package testkit

import (
	"github.com/huykingsofm/project-b/platform/sim"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// What follows is for the tests of the engines that are removed while the game runs. The game
// removes none: no action of it removes a zonelayer. A test makes the world do it with
// RemoveLayerCommand, which stands in for the rule of a game. The zonelayers that are made while
// the game runs are made by the commands of the game itself (command.WorldZoneLayerAddCommand).

const (
	removeLayerCommandID sim.CommandID = 9_000_002
	removeLayerActionID  sim.ActionID  = 9_000_002
)

// RemoveLayerCommand asks the world to forget a zonelayer and the cluster to remove its engine.
type RemoveLayerCommand struct{ Layer sim.StateID }

func (RemoveLayerCommand) ID() sim.CommandID { return removeLayerCommandID }
func (c RemoveLayerCommand) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(c.Layer))
	return nil
}
func (c RemoveLayerCommand) Translate() []sim.Action[*worldstate.State] {
	return []sim.Action[*worldstate.State]{RemoveLayerAction{Layer: c.Layer}}
}
func (c RemoveLayerCommand) Clone() sim.Command[*worldstate.State] { return c }

func decodeRemoveLayerCommand(r *binary.Reader, _ codec.Version) (RemoveLayerCommand, error) {
	return RemoveLayerCommand{Layer: r.Uint[sim.StateID]()}, r.Err()
}

// RemoveLayerAction makes the world forget a zonelayer and asks for its engine to be removed.
type RemoveLayerAction struct{ Layer sim.StateID }

func (RemoveLayerAction) ID() sim.ActionID { return removeLayerActionID }
func (RemoveLayerAction) Priority() int16  { return 0 }
func (RemoveLayerAction) Actor() any       { return nil }
func (a RemoveLayerAction) Apply(
	_ *sim.Context, s *worldstate.State, r *sim.Registry[*worldstate.State],
	_ *sim.Plan[*worldstate.State],
) {
	if s.RemoveZoneLayer(a.Layer) {
		r.RemoveEngine(a.Layer, nil)
	}
}
func (a RemoveLayerAction) Clone() sim.Action[*worldstate.State] { return a }
func (a RemoveLayerAction) Encode(w *binary.Writer)              { w.Uvarint(uint64(a.Layer)) }

func decodeRemoveLayerAction(r *binary.Reader, _ codec.Version) (RemoveLayerAction, error) {
	return RemoveLayerAction{Layer: r.Uint[sim.StateID]()}, r.Err()
}
