// Package world holds the state of the world: the private messages between players. It follows the
// same rules as the state of a zonelayer: data and nothing else, changed only through its own methods,
// with events of its own.
package world

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state/world/event"
	"github.com/huykingsofm/project-b/server/internal/shared/table"
)

// PrivateChatMessage is something a player sent privately to another player. A private message is
// between players, not between characters: a character is not always played by a player.
type PrivateChatMessage struct {
	FromID state.PlayerID
	ToID   state.PlayerID
	Text   string
	Tick   uint64
}

// Index says by which columns a message is found in a table of messages: who sent it and who it is
// for.
func (m PrivateChatMessage) Index() map[string]any {
	return map[string]any{"from": m.FromID, "to": m.ToID}
}

// chatKeep is how many private messages the world keeps.
const chatKeep = 1000

// State is the state of the world.
type State struct {
	// Data.
	ID      state.WorldID
	Tick    uint64
	Private table.Table[PrivateChatMessage]

	// Events: not data, never cloned.
	events state.Events[event.Event]
}

// NewState makes the state of an empty world.
func NewState(worldID state.WorldID) *State { return &State{ID: worldID} }

// Clone returns a copy of the data, with an empty event.
func (s *State) Clone() *State {
	return &State{ID: s.ID, Tick: s.Tick, Private: s.Private.Clone()}
}

// SetTick sets the tick that the state is at.
func (s *State) SetTick(tick uint64) { s.Tick = tick }

// TakeEvents returns the events of the tick and empties them.
func (s *State) TakeEvents() []event.Event { return s.events.Take() }

// SendPrivateMessage records a private message between two players and tells the clients.
func (s *State) SendPrivateMessage(m PrivateChatMessage) {
	s.Private.Add(m)
	s.Private.Trim(chatKeep)
	s.events.Add(event.PrivateChatMessageSent{FromID: m.FromID, ToID: m.ToID, Text: m.Text, Tick: m.Tick})
}
