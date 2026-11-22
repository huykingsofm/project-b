package event

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// CharacterNoticed tells a character something, as a text key with arguments that the client
// translates.
type CharacterNoticed struct {
	CharacterID state.CharacterID
	Key         string
	Args        []any
}

func (CharacterNoticed) ID() ID { return CharacterNoticedID }

// Audience is the character that is told, and nobody else.
func (e CharacterNoticed) Audience() state.Audience {
	return state.AudienceCharacters(e.CharacterID)
}
