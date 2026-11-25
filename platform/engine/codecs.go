package engine

import (
	"github.com/huykingsofm/project-b/platform/configengine"
	"github.com/huykingsofm/project-b/platform/sim"
)

// NewCodecs makes codecs that know what every cluster has, the configuration engine, and nothing
// else: the game registers its own states, actions and commands in them.
func NewCodecs() (*sim.Codecs, error) {
	c := sim.NewCodecs()
	if err := configengine.Register(c); err != nil {
		return nil, err
	}
	return c, nil
}
