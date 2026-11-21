package event

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// EntityMoved says an entity went from one position to another.
type EntityMoved struct {
	EntityID state.EntityID
	From, To state.Vec
}

func (EntityMoved) ID() ID { return EntityMovedID }
