package sim

import "github.com/huykingsofm/project-b/shared/codec/binary"

// ActionMessage is a list of actions for one engine, whose state is of type S, that are put in its
// plan together, with what a failed delivery means. A message is delivered whole or bounced whole.
type ActionMessage[S State] struct {
	Header   Header
	To       StateID
	Actions  []Action[S]
	onFail   OnFail
	attempts int
	subject  any
}

// NewActionMessage makes a message for the engine with the ID, which has a state of type S.
func NewActionMessage[S State](to StateID, onFail OnFail, actions ...Action[S]) Message {
	return newActionMessage(to, onFail, actions)
}

func newActionMessage[S State](
	to StateID, onFail OnFail, actions []Action[S],
) *ActionMessage[S] {
	m := &ActionMessage[S]{To: to, Actions: actions, onFail: onFail}
	for _, a := range actions {
		if actor := a.Actor(); actor != nil {
			m.subject = actor
			break
		}
	}
	return m
}

var _ Message = (*ActionMessage[State])(nil)

// Dest, Head, Subject, OnFail and Attempts make an ActionMessage a Message.
func (m *ActionMessage[S]) Dest() StateID  { return m.To }
func (m *ActionMessage[S]) Head() Header   { return m.Header }
func (m *ActionMessage[S]) Subject() any   { return m.subject }
func (m *ActionMessage[S]) OnFail() OnFail { return m.onFail }
func (m *ActionMessage[S]) Attempts() int  { return m.attempts }

// Stamp returns a copy that carries the header.
func (m *ActionMessage[S]) Stamp(h Header) Message {
	c := *m
	c.Header = h
	return &c
}

// Clone returns a copy that shares nothing with the message: each action and the OnFail are copied
// with their Clone.
func (m *ActionMessage[S]) Clone() Message {
	c := *m
	c.Actions = make([]Action[S], len(m.Actions))
	for i, a := range m.Actions {
		c.Actions[i] = a.Clone()
	}
	if m.onFail != nil {
		c.onFail = m.onFail.Clone()
	}
	return &c
}

// Again returns a copy for one more attempt.
func (m *ActionMessage[S]) Again() Message {
	c := *m
	c.attempts++
	return &c
}

// Redirected returns a copy for another engine.
func (m *ActionMessage[S]) Redirected(to StateID) Message {
	c := *m
	c.To = to
	return &c
}

// Encode writes the message whole: where it goes (its ID says the type of state it is for), who
// sent it, how many times it was tried, its OnFail and its actions.
func (m *ActionMessage[S]) Encode(w *binary.Writer) error {
	start := len(w.B)
	w.Uvarint(uint64(m.To))
	m.Header.Encode(w)
	w.Uvarint(uint64(m.attempts))
	if err := EncodeOnFail(w, m.onFail); err != nil {
		w.B = w.B[:start]
		return err
	}
	w.Uvarint(uint64(len(m.Actions)))
	for _, a := range m.Actions {
		if err := EncodeAction(w, a); err != nil {
			w.B = w.B[:start]
			return err
		}
	}
	return nil
}
