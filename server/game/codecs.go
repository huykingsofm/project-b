package game

import (
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/command"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/errs"
)

// NewCodecsAndEngines makes the codecs of the game and the registry that makes its engines: every
// state (and, with it, the way to make its engine), action, command and kind of OnFail, so that
// what the engines wrote can be read back and the engines can be made again.
func NewCodecsAndEngines() (*sim.Codecs, *engine.EngineRegistry, error) {
	c := sim.NewCodecs()
	engines := engine.NewEngineRegistry()
	err := errs.Join(
		engines.Register(c, worldstate.DecodeState),
		engines.Register(c, zonestate.DecodeState),
		engines.Register(c, zonelayerstate.DecodeState),
		onfail.Register(c),
		action.Register(c),
		command.Register(c),
	)
	if err != nil {
		return nil, nil, err
	}
	return c, engines, nil
}
