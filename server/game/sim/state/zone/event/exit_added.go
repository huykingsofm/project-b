package event

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
)

// ExitAdded says that an exit now stands in the zone: its entity, the area of the zone that leads
// out, the zone it leads to and where a character comes out there.
type ExitAdded struct {
	EntityID state.EntityID
	Area     state.Rect
	ToZoneID sim.StateID
	Arrival  state.Vec
}

func (ExitAdded) ID() ID { return ExitAddedID }

// Audience is everyone in the zone.
func (ExitAdded) Audience() state.Audience { return state.AudienceEveryone() }
