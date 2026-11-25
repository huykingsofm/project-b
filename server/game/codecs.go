package game

import (
	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/command"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/errs"
)

// NewCodecs makes the codecs of the game: every state, action, command and kind of OnFail, so that
// what the engines wrote can be read back. The map of a zone is not written with its state; tilesOf
// gives the map of the zone with an ID when a zone is read.
func NewCodecs(tilesOf func(zoneID sim.StateID) *tilemap.TileMap) (*sim.Codecs, error) {
	c, err := engine.NewCodecs()
	if err != nil {
		return nil, err
	}
	err = errs.Join(
		c.RegisterState(worldstate.DecodeState),
		c.RegisterState(
			func(r *binary.Reader, version codec.Version) (*zonestate.State, error) {
				return zonestate.DecodeState(r, version, tilesOf)
			},
		),
		c.RegisterState(zonelayerstate.DecodeState),
		onfail.Register(c),
		action.Register(c),
		command.Register(c),
	)
	if err != nil {
		return nil, err
	}
	return c, nil
}
