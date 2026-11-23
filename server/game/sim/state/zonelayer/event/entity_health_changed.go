package event

import (
	"github.com/huykingsofm/project-b/server/game/sim/state"
)

// EntityHealthChanged says the health of an entity changed by Change (negative for damage).
type EntityHealthChanged struct {
	EntityID state.EntityID
	Change   int32
	Health   int32
	At       state.Vec // where the entity is
	Radius   int32     // of the entity
}

func (EntityHealthChanged) ID() ID { return EntityHealthChangedID }

// Audience is whoever can see any part of the entity.
func (e EntityHealthChanged) Audience() state.Audience {
	return state.AudienceInSightOf(state.RectCovering(e.At, e.At).Grown(e.Radius))
}
