package configengine

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/errs"
)

// Register registers the state, the command and the action of the configuration engine with the
// codecs, so a checkpoint and an event log of it can be read back. A cluster needs it before it is
// built (see engine.NewCodecs).
func Register(c *sim.Codecs) error {
	return errs.Join(
		c.RegisterState(DecodeState),
		c.RegisterCommand(decodeUpdateConfigCommand),
		c.RegisterAction(decodeUpdateConfigAction),
	)
}
