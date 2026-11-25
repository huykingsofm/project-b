package sim

import (
	"math"

	"github.com/huykingsofm/project-b/shared/errs"
)

// ConfigStateType is the type of the state of the configuration engine, the last of the types.
const ConfigStateType StateType = math.MaxUint8

// ConfigStateID is the ID of the state of the configuration engine, which every cluster has once:
// the engines read the configuration from the copy it publishes.
var ConfigStateID = NewStateID(ConfigStateType, 1)

// The IDs of the command and the action of the configuration engine: the last of each register,
// as its state is the last of the types, so the game's own numbers, which count up from 1, never
// meet them.
const (
	UpdateConfigCommandID CommandID = math.MaxUint32
	UpdateConfigActionID  ActionID  = math.MaxUint32
)

// registerConfig registers the state, the command and the action of the configuration engine,
// which every Codecs knows, so a checkpoint and an event log of it can be read back.
func registerConfig(c *Codecs) error {
	return errs.Join(
		c.RegisterState(DecodeConfigState),
		c.RegisterCommand(decodeUpdateConfigCommand),
		c.RegisterAction(decodeUpdateConfigAction),
	)
}
