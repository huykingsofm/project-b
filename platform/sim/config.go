package sim

import (
	"math"

	"github.com/huykingsofm/project-b/platform/config"
)

// ConfigStateType is the type of the state of the configuration engine, the last of the types.
const ConfigStateType StateType = math.MaxUint8

// ConfigStateID is the ID of the state of the configuration engine, which every cluster has once:
// the engines read the configuration from the copy it publishes.
var ConfigStateID = NewStateID(ConfigStateType, 1)

// ConfigReader is what the state of the configuration engine gives out: the configuration as a
// snapshot that no one changes. A state that is one is where an engine reads its configuration
// from, itself if it is the one.
type ConfigReader interface {
	Snapshot() config.ReadOnlySnapshot
}

// ConfigOf returns the configuration that the state read from a StableReader holds, or the defaults
// when no engine published one: the configuration engine has not run yet, or the reader does not
// find it.
func ConfigOf(r StableReader) config.ReadOnlySnapshot {
	if r != nil {
		if st, err := r.Read(ConfigStateID); err == nil {
			if cr, ok := st.(ConfigReader); ok {
				return cr.Snapshot()
			}
		}
	}
	return config.Defaults()
}
