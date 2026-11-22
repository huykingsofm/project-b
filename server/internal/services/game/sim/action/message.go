package action

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
)

// ScopedMessage is a list of actions for one engine, that are put in its plan together, with what a
// failed delivery means. A message is delivered whole or bounced whole.
type ScopedMessage[A Action] struct {
	Header   message.Header
	To       message.Address
	Actions  []A
	onFail   message.OnFail
	attempts int
	subject  any
}

func newScopedMessage[A Action](
	to message.Address, onFail message.OnFail, actions []A,
) *ScopedMessage[A] {
	m := &ScopedMessage[A]{To: to, Actions: actions, onFail: onFail}
	for _, a := range actions {
		if who, ok := any(a).(Acting); ok {
			m.subject = who.Actor()
			break
		}
	}
	return m
}

var (
	_ message.Message = (*ScopedMessage[WorldAction])(nil)
	_ message.Message = (*ScopedMessage[ZoneAction])(nil)
	_ message.Message = (*ScopedMessage[ZoneLayerAction])(nil)
)

// Dest, Head, Subject, OnFail and Attempts make a ScopedMessage a message.Message.
func (m *ScopedMessage[A]) Dest() message.Address  { return m.To }
func (m *ScopedMessage[A]) Head() message.Header   { return m.Header }
func (m *ScopedMessage[A]) Subject() any           { return m.subject }
func (m *ScopedMessage[A]) OnFail() message.OnFail { return m.onFail }
func (m *ScopedMessage[A]) Attempts() int          { return m.attempts }

// Stamp returns a copy that carries the header.
func (m *ScopedMessage[A]) Stamp(h message.Header) message.Message {
	c := *m
	c.Header = h
	return &c
}

// Again returns a copy for one more attempt.
func (m *ScopedMessage[A]) Again() message.Message {
	c := *m
	c.attempts++
	return &c
}

// Redirected returns a copy for another engine of the same scope.
func (m *ScopedMessage[A]) Redirected(to message.Address) message.Message {
	c := *m
	c.To = to
	return &c
}

// MessageToZoneLayer makes a message for a zonelayer.
func MessageToZoneLayer(
	zoneLayerID state.ZoneLayerID, onFail message.OnFail, actions ...ZoneLayerAction,
) message.Message {
	return newScopedMessage(message.ZoneLayerAddress(zoneLayerID), onFail, actions)
}

// MessageToZone makes a message for a zone.
func MessageToZone(
	zoneID state.ZoneID, onFail message.OnFail, actions ...ZoneAction,
) message.Message {
	return newScopedMessage(message.ZoneAddress(zoneID), onFail, actions)
}

// MessageToWorld makes a message for a world.
func MessageToWorld(
	worldID state.WorldID, onFail message.OnFail, actions ...WorldAction,
) message.Message {
	return newScopedMessage(message.WorldAddress(worldID), onFail, actions)
}
