package event

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// ItemDisappeared says an item is no longer on the ground: it was picked up.
type ItemDisappeared struct {
	EntityID state.EntityID
	At       state.Vec // where the item lay
}

func (ItemDisappeared) ID() ID { return ItemDisappearedID }

// Audience is whoever could see the place where the item lay.
func (e ItemDisappeared) Audience() state.Audience {
	return state.AudienceInSightOf(state.RectCovering(e.At, e.At))
}
