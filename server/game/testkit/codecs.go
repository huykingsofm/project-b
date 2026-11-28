package testkit

import (
	"github.com/huykingsofm/project-b/platform/sim"
)

// registerFixtures registers the commands and the actions of the zonelayers that a test retires.
func registerFixtures(c *sim.Codecs) error {
	if err := c.RegisterCommand(decodeRetireLayerCommand); err != nil {
		return err
	}
	return c.RegisterAction(decodeRetireLayerAction)
}
