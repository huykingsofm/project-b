package event

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// EntityMoved says an entity went from one position to another.
type EntityMoved struct {
	EntityID state.EntityID
	From, To state.Vec
	Radius   int32 // of the entity
}

func (EntityMoved) ID() ID { return EntityMovedID }

// Audience is whoever can see any part of the entity on the way it went, from where it was to where it is.
func (e EntityMoved) Audience() state.Audience {
	return state.AudienceInSightOf(state.RectCovering(e.From, e.To).Grown(e.Radius))
}
