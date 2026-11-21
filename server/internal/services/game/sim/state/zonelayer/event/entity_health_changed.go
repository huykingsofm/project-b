package event

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// EntityHealthChanged says the health of an entity changed by Change (negative for damage).
type EntityHealthChanged struct {
	EntityID state.EntityID
	Change   int32
	Health   int32
}

func (EntityHealthChanged) ID() ID { return EntityHealthChangedID }
