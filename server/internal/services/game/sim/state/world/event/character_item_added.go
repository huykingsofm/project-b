package event

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// CharacterItemAdded says a character got some items of a kind in its inventory.
type CharacterItemAdded struct {
	CharacterID state.CharacterID
	ItemID      state.ItemID
	Count       uint32
}

func (CharacterItemAdded) ID() ID { return CharacterItemAddedID }

// Audience is the character that got the items, and nobody else.
func (e CharacterItemAdded) Audience() state.Audience { return state.AudienceCharacters(e.CharacterID) }
