package zonelayer

import "github.com/huykingsofm/project-b/server/internal/services/game/sim/state"

// Entity is something that stands in a zonelayer. Which character it is the body of is in the state,
// not here (see State.EntityOfCharacter).
type Entity struct {
	ID  state.EntityID
	Pos state.Vec
}
