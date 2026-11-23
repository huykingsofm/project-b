package game

import (
	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/ids"
)

// worldDirectory is what a router asks why a message has no engine to go to: the state of the
// world says what each state ID is, and the router reads the copy of it that the world engine
// published at the end of its last tick, so it takes no lock and never waits for the world.
type worldDirectory struct {
	stable  engine.StableStore
	worldID sim.StateID
}

var _ sim.StateDirectory = worldDirectory{}

// StateStatus says what a state ID is, by the world as it was at the end of its last tick. Until
// the world has published, no ID is given.
func (d worldDirectory) StateStatus(id sim.StateID) ids.Status {
	published, err := d.stable.View(d.worldID, replay.NopLog{}).Read(d.worldID)
	if err != nil {
		return ids.NotGiven
	}
	if directory, ok := published.(sim.StateDirectory); ok {
		return directory.StateStatus(id)
	}
	return ids.NotGiven
}
