package event

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// EntityDied says an entity has no health left.
type EntityDied struct {
	EntityID state.EntityID
	At       state.Vec // where the entity was
	Radius   int32     // of the entity
}

func (EntityDied) ID() ID { return EntityDiedID }

// Audience is whoever can see any part of the entity where it died.
func (e EntityDied) Audience() state.Audience {
	return state.AudienceInSightOf(state.RectCovering(e.At, e.At).Grown(e.Radius))
}
