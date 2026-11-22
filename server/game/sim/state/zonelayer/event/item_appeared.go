package event

import (
	"github.com/huykingsofm/project-b/server/game/sim/state"
)

// ItemAppeared says an item is now lying on the ground at a position.
type ItemAppeared struct {
	EntityID state.EntityID
	ItemID   state.ItemID
	Count    uint32
	Pos      state.Vec
}

func (ItemAppeared) ID() ID { return ItemAppearedID }

// Audience is whoever can see the place where the item lies.
func (e ItemAppeared) Audience() state.Audience {
	return state.AudienceInSightOf(state.RectCovering(e.Pos, e.Pos))
}
