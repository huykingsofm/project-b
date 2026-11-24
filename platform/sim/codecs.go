package sim

import (
	"fmt"

	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ActionDecoder, CommandDecoder, StateDecoder and OnFailDecoder read one kind of what the codecs
// register, of the data version given. The decoder of an OnFail that holds a message is given the
// function that reads one.
type (
	ActionDecoder  func(r *binary.Reader, version codec.Version) (BaseAction, error)
	CommandDecoder func(r *binary.Reader, version codec.Version) (BaseCommand, error)
	StateDecoder   func(r *binary.Reader, version codec.Version) (State, error)
	OnFailDecoder  func(
		r *binary.Reader, version codec.Version, nested MessageDecoder,
	) (OnFail, error)
	stateMessageDecoder func(
		to StateID, r *binary.Reader, version codec.Version, nested MessageDecoder,
	) (Message, error)
)

// maxNesting is how deep messages may go inside messages (a message whose OnFail holds a message).
const maxNesting = 8

// Codecs holds what is needed to read back what the engines write: the actions, the commands, the
// states and the kinds of OnFail of a game, each in a register of its own, and the readers of the
// messages of each type of state. Writing needs none of it, because everything that is written
// says what it is itself. The game fills it once (see RegisterAction and the others), the engines
// and the replay only read it.
type Codecs struct {
	Actions  *codec.Registry[ActionID, BaseAction, ActionDecoder]
	Commands *codec.Registry[CommandID, BaseCommand, CommandDecoder]
	States   *codec.Registry[StateType, State, StateDecoder]
	OnFails  *codec.Registry[OnFailID, OnFail, OnFailDecoder]
	messages map[StateType]stateMessageDecoder
}

// NewCodecs makes empty codecs.
func NewCodecs() *Codecs {
	return &Codecs{
		Actions:  codec.New[ActionID, BaseAction, ActionDecoder](BaseAction.ID),
		Commands: codec.New[CommandID, BaseCommand, CommandDecoder](BaseCommand.ID),
		States:   codec.New[StateType, State, StateDecoder](State.Type),
		OnFails:  codec.New[OnFailID, OnFail, OnFailDecoder](OnFail.ID),
		messages: map[StateType]stateMessageDecoder{},
	}
}

// RegisterState registers a type of state: decode reads it, and the type of state and its Go type
// are the ones of S. It also makes the messages for the states of this type readable, which carry
// actions of S. It is an error when the type or the Go type is already registered. The type of a
// state is a constant of its kind, so it is read from the zero value of S, which is not used for
// anything else.
func (c *Codecs) RegisterState[S State](
	decode func(r *binary.Reader, version codec.Version) (S, error),
) error {
	var prototype S
	err := c.States.Register(
		prototype,
		func(r *binary.Reader, version codec.Version) (State, error) {
			s, err := decode(r, version)
			if err != nil {
				return nil, err
			}
			return s, nil
		},
	)
	if err != nil {
		return err
	}
	c.messages[prototype.Type()] = c.decodeActionMessage[S]
	return nil
}

// RegisterAction registers an action A of the states of type S: decode reads it, and its ID and
// its Go type are the ones of A. It is an error when its ID or its Go type is already registered.
// The ID is read from the zero value of A, so the ID method of an action does not read its fields.
func (c *Codecs) RegisterAction[S State, A Action[S]](
	decode func(r *binary.Reader, version codec.Version) (A, error),
) error {
	var prototype A
	return c.Actions.Register(
		prototype,
		func(r *binary.Reader, version codec.Version) (BaseAction, error) {
			a, err := decode(r, version)
			if err != nil {
				return nil, err
			}
			return a, nil
		},
	)
}

// RegisterCommand registers a command C of the states of type S, as RegisterAction does an action.
func (c *Codecs) RegisterCommand[S State, C Command[S]](
	decode func(r *binary.Reader, version codec.Version) (C, error),
) error {
	var prototype C
	return c.Commands.Register(
		prototype,
		func(r *binary.Reader, version codec.Version) (BaseCommand, error) {
			cmd, err := decode(r, version)
			if err != nil {
				return nil, err
			}
			return cmd, nil
		},
	)
}

// RegisterOnFail registers a kind of OnFail. It is an error when its ID or its Go type is already
// registered.
func (c *Codecs) RegisterOnFail(prototype OnFail, decode OnFailDecoder) error {
	return c.OnFails.Register(prototype, decode)
}

// EncodeAction writes an action: its ID and its fields.
func EncodeAction(w *binary.Writer, a BaseAction) error {
	if a == nil {
		return fmt.Errorf("%w: nil", ErrNoCodec)
	}
	e, ok := a.(Encoder)
	if !ok {
		return fmt.Errorf("%w: %T has no Encode", ErrNoCodec, a)
	}
	w.Uvarint(uint64(a.ID()))
	e.Encode(w)
	return nil
}

// DecodeAction reads what EncodeAction wrote, of the data version given.
func (c *Codecs) DecodeAction(
	r *binary.Reader, version codec.Version,
) (BaseAction, error) {
	code := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, err
	}
	decode, ok := c.Actions.Lookup(ActionID(code))
	if !ok || code > 0xffff {
		return nil, fmt.Errorf("%w: action %d", ErrUnknownCode, code)
	}
	return decode(r, version)
}

// EncodeCommand writes a command: its ID and its fields. Nothing of a command that cannot be
// written is left in w.
func EncodeCommand(w *binary.Writer, c BaseCommand) error {
	if c == nil {
		return fmt.Errorf("%w: nil command", ErrNoCodec)
	}
	start := len(w.B)
	w.Uvarint(uint64(c.ID()))
	if err := c.Encode(w); err != nil {
		w.B = w.B[:start]
		return err
	}
	return nil
}

// DecodeCommand reads what EncodeCommand wrote, of the data version given. The command is of the
// type that was written, which a reader that knows the engine it is for asserts to a Command[S].
func (c *Codecs) DecodeCommand(
	r *binary.Reader, version codec.Version,
) (BaseCommand, error) {
	code := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, err
	}
	decode, ok := c.Commands.Lookup(CommandID(code))
	if !ok || code > 0xffff {
		return nil, fmt.Errorf("%w: command %d", ErrUnknownCode, code)
	}
	return decode(r, version)
}

// EncodeState writes a state: its type and its fields.
func EncodeState(w *binary.Writer, s State) error {
	if s == nil {
		return fmt.Errorf("%w: nil state", ErrNoCodec)
	}
	start := len(w.B)
	w.Uvarint(uint64(s.Type()))
	if err := s.Encode(w); err != nil {
		w.B = w.B[:start]
		return err
	}
	return nil
}

// DecodeState reads what EncodeState wrote, of the data version given.
func (c *Codecs) DecodeState(r *binary.Reader, version codec.Version) (State, error) {
	code := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, err
	}
	return c.DecodeStateOfType(StateType(code), r, version)
}

// DecodeStateOfType reads the fields of a state of the type, after its type was read.
func (c *Codecs) DecodeStateOfType(
	t StateType, r *binary.Reader, version codec.Version,
) (State, error) {
	decode, ok := c.States.Lookup(t)
	if !ok {
		return nil, fmt.Errorf("%w: state type %d", ErrUnknownCode, t)
	}
	return decode(r, version)
}

// EncodeOnFail writes an OnFail: its ID and what it holds. Nothing of an OnFail that cannot be
// written is left in w.
func EncodeOnFail(w *binary.Writer, f OnFail) error {
	if f == nil {
		return fmt.Errorf("%w: nil OnFail", ErrNoCodec)
	}
	start := len(w.B)
	w.Uvarint(uint64(f.ID()))
	if err := f.Encode(w); err != nil {
		w.B = w.B[:start]
		return err
	}
	return nil
}

// EncodeMessage writes a message whole. A message whose OnFail holds a message that holds a message
// and so on deeper than DecodeMessage reads is refused: what is written must be readable.
func EncodeMessage(w *binary.Writer, m Message) error {
	if m == nil {
		return fmt.Errorf("%w: nil message", ErrNoCodec)
	}
	if nesting(m) > maxNesting {
		return fmt.Errorf(
			"%w: messages inside messages go deeper than %d", binary.ErrBadValue, maxNesting,
		)
	}
	return m.Encode(w)
}

func nesting(m Message) int {
	if n, ok := m.OnFail().(NestedOnFail); ok && n.Nested() != nil {
		return 1 + nesting(n.Nested())
	}
	return 0
}

// DecodeMessage reads a message that EncodeMessage wrote, of the data version given.
func (c *Codecs) DecodeMessage(
	r *binary.Reader, version codec.Version,
) (Message, error) {
	return c.decodeMessage(r, version, 0)
}

func (c *Codecs) decodeMessage(
	r *binary.Reader, version codec.Version, depth int,
) (Message, error) {
	if depth > maxNesting {
		return nil, fmt.Errorf(
			"%w: messages inside messages go deeper than %d", binary.ErrBadValue, maxNesting,
		)
	}
	to := r.Uint[StateID]()
	if err := r.Err(); err != nil {
		return nil, err
	}
	decode, ok := c.messages[to.Type()]
	if !ok {
		return nil, fmt.Errorf("%w: state type %d", ErrUnknownCode, to.Type())
	}
	return decode(to, r, version, func(
		r *binary.Reader, version codec.Version,
	) (Message, error) {
		return c.decodeMessage(r, version, depth+1)
	})
}

// decodeActionMessage reads the rest of a message for states of type S, after its type.
func (c *Codecs) decodeActionMessage[S State](
	to StateID, r *binary.Reader, version codec.Version, nested MessageDecoder,
) (Message, error) {
	header := DecodeHeader(r)
	attempts := r.Uint[uint8]()
	onFail, err := c.decodeOnFail(r, version, nested)
	if err != nil {
		return nil, err
	}
	actions, err := c.DecodeActions[S](r, version)
	if err != nil {
		return nil, err
	}
	m := newActionMessage(to, onFail, actions)
	m.Header = header
	m.attempts = int(attempts)
	return m, r.Err()
}

func (c *Codecs) decodeOnFail(
	r *binary.Reader, version codec.Version, nested MessageDecoder,
) (OnFail, error) {
	code := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, err
	}
	decode, ok := c.OnFails.Lookup(OnFailID(code))
	if !ok || code > 0xffff {
		return nil, fmt.Errorf("%w: OnFail %d", ErrUnknownCode, code)
	}
	return decode(r, version, nested)
}

// DecodeOnFail reads what EncodeOnFail wrote, of the data version given. A kind that holds a
// message reads it with nested, which may be nil for a reader that expects none.
func (c *Codecs) DecodeOnFail(
	r *binary.Reader, version codec.Version, nested MessageDecoder,
) (OnFail, error) {
	return c.decodeOnFail(r, version, nested)
}

// EncodeActions writes a list of actions: how many, and each. It returns an error, and the writer
// holds a partial list, when an action cannot be written.
func EncodeActions[S State](w *binary.Writer, actions []Action[S]) error {
	w.Uvarint(uint64(len(actions)))
	for _, a := range actions {
		if err := EncodeAction(w, a); err != nil {
			return err
		}
	}
	return nil
}

// DecodeActions reads a list of actions of the states of type S, as EncodeActions wrote it. An
// action that is not an Action[S] is an error.
func (c *Codecs) DecodeActions[S State](
	r *binary.Reader, version codec.Version,
) ([]Action[S], error) {
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, err
	}
	if n > uint64(r.Left()) { // an action takes at least one byte
		return nil, fmt.Errorf("%w: %d actions", binary.ErrShort, n)
	}
	var out []Action[S] // nil, not empty, when there is none, as a list made in code has it
	for range n {
		decoded, err := c.DecodeAction(r, version)
		if err != nil {
			return nil, err
		}
		a, ok := decoded.(Action[S])
		if !ok {
			return nil, fmt.Errorf(
				"%w: %T is not an action of the type of state %d",
				ErrUnknownCode, decoded, StateTypeOf[S](),
			)
		}
		out = append(out, a)
	}
	return out, nil
}
