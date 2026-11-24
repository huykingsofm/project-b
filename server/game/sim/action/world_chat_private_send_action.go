package action

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// WorldChatPrivateSendAction is a private message from one player to another. It is between
// players, not between characters (a character is not always played by a player), and it is the
// world that manages it: it records it and tells the clients, and no zone or zonelayer takes part.
type WorldChatPrivateSendAction struct {
	FromID state.PlayerID
	ToID   state.PlayerID
	Text   string
}

func (a *WorldChatPrivateSendAction) ID() sim.ActionID { return WorldChatPrivateSendActionID }
func (a *WorldChatPrivateSendAction) Priority() int16  { return PriorityResolve }
func (a *WorldChatPrivateSendAction) Actor() any       { return a.FromID }

// Clone returns a copy that shares nothing with the action.
func (a *WorldChatPrivateSendAction) Clone() sim.Action[*worldstate.State] {
	c := *a
	return &c
}

// Apply records the message.
func (a *WorldChatPrivateSendAction) Apply(
	c *sim.Context, s *worldstate.State, _ *sim.Registry[*worldstate.State],
	_ *sim.Plan[*worldstate.State],
) {
	s.SendPrivateMessage(
		worldstate.NewPrivateChatMessage(a.FromID, a.ToID, a.Text, c.Tick),
	)
}

// Encode writes the fields of the action, not its ID: EncodeAction writes the ID first.
func (a *WorldChatPrivateSendAction) Encode(w *binary.Writer) {
	w.Uvarint(uint64(a.FromID))
	w.Uvarint(uint64(a.ToID))
	w.String(a.Text)
}

// decodeWorldChatPrivateSendAction reads what Encode wrote.
func decodeWorldChatPrivateSendAction(
	r *binary.Reader, version codec.Version,
) (*WorldChatPrivateSendAction, error) {
	a := &WorldChatPrivateSendAction{
		FromID: r.Uint[state.PlayerID](),
		ToID:   r.Uint[state.PlayerID](),
		Text:   r.String(),
	}
	return a, r.Err()
}
