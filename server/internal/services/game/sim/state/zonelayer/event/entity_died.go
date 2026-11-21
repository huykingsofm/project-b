package event

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// EntityDied says an entity has no health left.
type EntityDied struct{ EntityID state.EntityID }

func (EntityDied) ID() ID { return EntityDiedID }
