package testkit

import (
	"github.com/huykingsofm/project-b/platform/sim"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// What follows is for the tests of the engines that are retired while the game runs. The game
// retires none: no action of it retires a zonelayer. A test makes the world do it with
// RetireLayerCommand, which stands in for the rule of a game. The zonelayers that are made while
// the game runs are made by the commands of the game itself (command.WorldZoneLayerAddCommand).

const (
	retireLayerCommandID sim.CommandID = 9_000_002
	retireLayerActionID  sim.ActionID  = 9_000_002
)

// RetireLayerCommand asks the world to forget a zonelayer and the cluster to retire its engine.
type RetireLayerCommand struct{ Layer sim.StateID }

func decodeRetireLayerCommand(r *binary.Reader, _ codec.Version) (RetireLayerCommand, error) {
	return RetireLayerCommand{Layer: r.Uint[sim.StateID]()}, r.Err()
}

func (RetireLayerCommand) ID() sim.CommandID { return retireLayerCommandID }

func (c RetireLayerCommand) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(c.Layer))
	return nil
}

func (c RetireLayerCommand) Translate() []sim.Action[*worldstate.State] {
	return []sim.Action[*worldstate.State]{RetireLayerAction{Layer: c.Layer}}
}

func (c RetireLayerCommand) Clone() sim.Command[*worldstate.State] { return c }

// RetireLayerAction makes the world forget a zonelayer and asks for its engine to be retired.
type RetireLayerAction struct{ Layer sim.StateID }

func decodeRetireLayerAction(r *binary.Reader, _ codec.Version) (RetireLayerAction, error) {
	return RetireLayerAction{Layer: r.Uint[sim.StateID]()}, r.Err()
}

func (RetireLayerAction) ID() sim.ActionID { return retireLayerActionID }

func (RetireLayerAction) Priority() int16 { return 0 }

func (RetireLayerAction) Actor() any { return nil }

func (a RetireLayerAction) Apply(
	_ *sim.Context, s *worldstate.State, r *sim.ActionRegistry[*worldstate.State],
	_ *sim.Plan[*worldstate.State],
) {
	if s.RemoveZoneLayer(a.Layer) {
		r.RequestRetireEngine(a.Layer, nil)
	}
}

func (a RetireLayerAction) Clone() sim.Action[*worldstate.State] { return a }

func (a RetireLayerAction) Encode(w *binary.Writer) { w.Uvarint(uint64(a.Layer)) }
