package runner

import (
	"errors"
	"fmt"

	"github.com/huykingsofm/project-b/server/internal/services/game/engine"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/command"
)

// Engine is what the runner needs of an engine: to run a tick, to say which engine it is, and to be
// given a command that was read from a log.
type Engine interface {
	Step()
	Address() message.Address
	// SendCommand sends the engine a command, which a log holds without the type of its scope. It
	// returns an error for a command of another scope and for a full queue.
	SendCommand(c command.Command) error
}

// worldEngine, zoneEngine and zoneLayerEngine make the engine of a scope an Engine: each gives a
// command that was read from a log to the Send of the engine, as the command of its scope that it
// has to be.
type (
	worldEngine     struct{ *engine.WorldEngine }
	zoneEngine      struct{ *engine.ZoneEngine }
	zoneLayerEngine struct{ *engine.ZoneLayerEngine }
)

var (
	_ Engine = worldEngine{}
	_ Engine = zoneEngine{}
	_ Engine = zoneLayerEngine{}
)

func (e worldEngine) SendCommand(c command.Command) error { return sendCommand(c, e.Send, "world") }

func (e zoneEngine) SendCommand(c command.Command) error { return sendCommand(c, e.Send, "zone") }

func (e zoneLayerEngine) SendCommand(c command.Command) error {
	return sendCommand(c, e.Send, "zonelayer")
}

// sendCommand sends a command that a log holds to the Send of an engine, as the command of its
// scope that it has to be.
func sendCommand[C command.Command](c command.Command, send func(C) bool, scope string) error {
	typed, ok := c.(C)
	if !ok {
		return fmt.Errorf("%T is not a command of a %s", c, scope)
	}
	if !send(typed) {
		return errors.New("the queue of commands is full")
	}
	return nil
}

// enginesOf returns every engine of the cluster, in the order that Step of the cluster runs them:
// the zonelayers, the zones, the world.
func enginesOf(c *engine.Cluster) []Engine {
	var out []Engine
	for _, l := range c.ZoneLayers() {
		out = append(out, zoneLayerEngine{l})
	}
	for _, z := range c.Zones() {
		out = append(out, zoneEngine{z})
	}
	return append(out, worldEngine{c.World})
}
