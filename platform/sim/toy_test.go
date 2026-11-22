package sim

import "github.com/huykingsofm/project-b/shared/binarycodec"

// A toy game, for the tests of the mechanism: a state that counts, and the smallest actions and
// commands that the codecs and the plan can be tried on. Nothing of a real game is needed here.

// toyType is the type of the toy state.
const toyType StateType = 1

// toyState is a state with an ID, a tick and a total.
type toyState struct {
	id    StateID
	tick  uint64
	Total int64
}

func (s *toyState) ID() StateID      { return s.id }
func (*toyState) Type() StateType    { return toyType }
func (*toyState) String() string     { return "toy" }
func (s *toyState) Tick() uint64     { return s.tick }
func (s *toyState) SetTick(t uint64) { s.tick = t }
func (s *toyState) Clone() State     { c := *s; return &c }
func (s *toyState) Hash() uint64 {
	h := NewHasher()
	h.U64(uint64(s.id))
	h.U64(uint64(s.Total))
	return h.Sum()
}
func (s *toyState) TakeEvents() []Event { return nil }
func (s *toyState) Encode(w *binarycodec.Writer) error {
	w.Uvarint(uint64(s.id))
	w.Uvarint(s.tick)
	w.Varint(s.Total)
	return nil
}

func decodeToyState(r *binarycodec.Reader, _ binarycodec.Version) (*toyState, error) {
	s := &toyState{id: StateID(r.Uvarint()), tick: r.Uvarint(), Total: r.Varint()}
	return s, r.Err()
}

// otherState is a state of another type, for what must not be taken for a toyState.
type otherState struct{ toyState }

const otherType StateType = 2

func (*otherState) Type() StateType { return otherType }
func (*otherState) String() string  { return "other" }
func (s *otherState) Clone() State {
	c := *s
	return &c
}

func decodeOtherState(r *binarycodec.Reader, v binarycodec.Version) (*otherState, error) {
	s, err := decodeToyState(r, v)
	if err != nil {
		return nil, err
	}
	return &otherState{*s}, nil
}

// testID is the ID of the actions of these tests; it is not one of a register.
const testID ActionID = 60000

// anonymousAction has an ID and a priority and no author. It does nothing when it runs.
type anonymousAction struct {
	name     string
	priority int16
}

func (a *anonymousAction) ID() ActionID    { return testID }
func (a *anonymousAction) Priority() int16 { return a.priority }
func (a *anonymousAction) Apply(*Context, *toyState, *Registry[*toyState], *Plan[*toyState]) {
}

func (a *anonymousAction) Encode(w *binarycodec.Writer) {
	w.String(a.name)
	w.Varint(int64(a.priority))
}

func decodeAnonymousAction(
	r *binarycodec.Reader, _ binarycodec.Version,
) (Action[*toyState], error) {
	a := &anonymousAction{name: r.String(), priority: int16(r.Varint())}
	return a, r.Err()
}

// authoredAction has an author too.
type authoredAction struct {
	anonymousAction
	author int
}

func (a *authoredAction) Actor() any { return a.author }

// addAction adds to the total of the toy state.
type addAction struct{ N int64 }

const addID ActionID = 60002

func (a *addAction) ID() ActionID    { return addID }
func (a *addAction) Priority() int16 { return 0 }
func (a *addAction) Apply(_ *Context, s *toyState, _ *Registry[*toyState], _ *Plan[*toyState]) {
	s.Total += a.N
}
func (a *addAction) Encode(w *binarycodec.Writer) { w.Varint(a.N) }

func decodeAddAction(r *binarycodec.Reader, _ binarycodec.Version) (Action[*toyState], error) {
	a := &addAction{N: r.Varint()}
	return a, r.Err()
}

// otherAction is an action of the other type of state.
type otherAction struct{}

const otherActionID ActionID = 60003

func (*otherAction) ID() ActionID    { return otherActionID }
func (*otherAction) Priority() int16 { return 0 }
func (*otherAction) Apply(*Context, *otherState, *Registry[*otherState], *Plan[*otherState]) {
}
func (*otherAction) Encode(*binarycodec.Writer) {}

func decodeOtherAction(
	_ *binarycodec.Reader, _ binarycodec.Version,
) (Action[*otherState], error) {
	return &otherAction{}, nil
}

// addCommand is a command that asks for an add.
type addCommand struct{ N int64 }

const addCommandID CommandID = 60000

func (addCommand) ID() CommandID { return addCommandID }
func (c addCommand) Encode(w *binarycodec.Writer) error {
	w.Varint(c.N)
	return nil
}
func (c addCommand) Translate() []Action[*toyState] {
	return []Action[*toyState]{&addAction{N: c.N}}
}
func (c addCommand) Clone() Command[*toyState] { return c }

func decodeAddCommand(r *binarycodec.Reader, _ binarycodec.Version) (Command[*toyState], error) {
	c := addCommand{N: r.Varint()}
	return c, r.Err()
}

// nameOf is the name of an anonymousAction or an authoredAction.
func nameOf(a Action[*toyState]) string {
	switch a := a.(type) {
	case *anonymousAction:
		return a.name
	case *authoredAction:
		return a.name
	case *typedAction:
		return a.name
	}
	return ""
}

// toyCodecs are codecs with the toy state, its actions and its command registered.
func toyCodecs() (*Codecs, error) {
	c := NewCodecs()
	return c, Errors(
		c.RegisterState[*toyState](&toyState{}, decodeToyState),
		c.RegisterState[*otherState](&otherState{}, decodeOtherState),
		c.RegisterAction[*toyState](&anonymousAction{}, decodeAnonymousAction),
		c.RegisterAction[*toyState](&addAction{}, decodeAddAction),
		c.RegisterAction[*otherState](&otherAction{}, decodeOtherAction),
		c.RegisterCommand[*toyState](addCommand{}, decodeAddCommand),
		c.RegisterOnFail(testDrop{}, decodeTestDrop),
		c.RegisterOnFail(testCompensate{}, decodeTestCompensate),
	)
}

// testDrop and testCompensate are the kinds of OnFail of these tests: the real ones are in package
// onfail, which imports this package.
type testDrop struct{}

const (
	testDropID       OnFailID = 60001
	testCompensateID OnFailID = 60002
)

func (testDrop) ID() OnFailID                          { return testDropID }
func (testDrop) Handle(b Bouncer, _ Message, _ Reason) { b.Drop() }
func (testDrop) Encode(*binarycodec.Writer) error      { return nil }

func decodeTestDrop(
	*binarycodec.Reader, binarycodec.Version, MessageDecoder,
) (OnFail, error) {
	return testDrop{}, nil
}

type testCompensate struct{ With Message }

func (testCompensate) ID() OnFailID                            { return testCompensateID }
func (f testCompensate) Handle(b Bouncer, _ Message, _ Reason) { b.Resend(f.With) }
func (f testCompensate) Nested() Message                       { return f.With }
func (f testCompensate) Encode(w *binarycodec.Writer) error {
	if f.With == nil {
		return ErrNoCodec
	}
	return f.With.Encode(w)
}

func decodeTestCompensate(
	r *binarycodec.Reader, v binarycodec.Version, nested MessageDecoder,
) (OnFail, error) {
	m, err := nested(r, v)
	return testCompensate{With: m}, err
}
