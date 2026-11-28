package sim_test

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/errs"
)

// A toy game, for the tests of the mechanism: a state that counts, and the smallest actions and
// commands that the codecs and the plan can be tried on. Nothing of a real game is needed here.

// toyType is the type of the toy state.
const toyType sim.StateType = 0 // so a plain number is the ID of a toy state

// toyState is a state with a mark, a tick and a total.
type toyState struct {
	id    sim.StateID
	tick  uint64
	Total int64
}

func (*toyState) Type() sim.StateType { return toyType }
func (s *toyState) Tick() uint64      { return s.tick }
func (s *toyState) AdvanceTick()      { s.tick++ }
func (s *toyState) Clone() sim.State  { c := *s; return &c }
func (s *toyState) Hash() uint64 {
	h := sim.NewHasher()
	h.U64(uint64(s.id))
	h.U64(uint64(s.Total))
	return h.Sum()
}
func (s *toyState) TakeEvents() []sim.Event { return nil }
func (s *toyState) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(s.id))
	w.Uvarint(s.tick)
	w.Varint(s.Total)
	return nil
}

func decodeToyState(r *binary.Reader, _ codec.Version) (*toyState, error) {
	s := &toyState{id: sim.StateID(r.Uvarint()), tick: r.Uvarint(), Total: r.Varint()}
	return s, r.Err()
}

// otherState is a state of another type, for what must not be taken for a toyState.
type otherState struct{ toyState }

const otherType sim.StateType = 1

func (*otherState) Type() sim.StateType { return otherType }
func (s *otherState) Clone() sim.State {
	c := *s
	return &c
}

func decodeOtherState(r *binary.Reader, v codec.Version) (*otherState, error) {
	s, err := decodeToyState(r, v)
	if err != nil {
		return nil, err
	}
	return &otherState{*s}, nil
}

// testID is the ID of the actions of these tests; it is not one of a register.
const testID sim.ActionID = 60000

// anonymousAction has an ID and a priority and no author. It does nothing when it runs.
type anonymousAction struct {
	name     string
	priority int16
}

func (a anonymousAction) ID() sim.ActionID { return testID }
func (a anonymousAction) Priority() int16  { return a.priority }
func (a anonymousAction) Actor() any       { return nil }
func (a anonymousAction) Apply(*sim.Context, *toyState, *sim.ActionRegistry[*toyState],
	*sim.Plan[*toyState]) {
}
func (a anonymousAction) Clone() sim.Action[*toyState] { return a }

func (a anonymousAction) Encode(w *binary.Writer) {
	w.String(a.name)
	w.Varint(int64(a.priority))
}

func decodeAnonymousAction(
	r *binary.Reader, _ codec.Version,
) (anonymousAction, error) {
	a := anonymousAction{name: r.String(), priority: int16(r.Varint())}
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

const addID sim.ActionID = 60002

func (a addAction) ID() sim.ActionID { return addID }
func (a addAction) Priority() int16  { return 0 }
func (a addAction) Actor() any       { return nil }
func (a addAction) Apply(_ *sim.Context, s *toyState, _ *sim.ActionRegistry[*toyState],
	_ *sim.Plan[*toyState]) {
	s.Total += a.N
}
func (a addAction) Clone() sim.Action[*toyState] { return a }
func (a addAction) Encode(w *binary.Writer)      { w.Varint(a.N) }

func decodeAddAction(r *binary.Reader, _ codec.Version) (addAction, error) {
	a := addAction{N: r.Varint()}
	return a, r.Err()
}

// otherAction is an action of the other type of state.
type otherAction struct{}

const otherActionID sim.ActionID = 60003

func (otherAction) ID() sim.ActionID { return otherActionID }
func (otherAction) Priority() int16  { return 0 }
func (otherAction) Actor() any       { return nil }
func (otherAction) Apply(*sim.Context, *otherState, *sim.ActionRegistry[*otherState],
	*sim.Plan[*otherState]) {
}
func (a otherAction) Clone() sim.Action[*otherState] { return a }
func (otherAction) Encode(*binary.Writer)            {}

func decodeOtherAction(
	_ *binary.Reader, _ codec.Version,
) (otherAction, error) {
	return otherAction{}, nil
}

// addCommand is a command that asks for an add.
type addCommand struct{ N int64 }

const addCommandID sim.CommandID = 60000

func (addCommand) ID() sim.CommandID { return addCommandID }
func (c addCommand) Encode(w *binary.Writer) error {
	w.Varint(c.N)
	return nil
}
func (c addCommand) Translate() []sim.Action[*toyState] {
	return []sim.Action[*toyState]{addAction{N: c.N}}
}
func (c addCommand) Clone() sim.Command[*toyState] { return c }

func decodeAddCommand(r *binary.Reader, _ codec.Version) (addCommand, error) {
	c := addCommand{N: r.Varint()}
	return c, r.Err()
}

// nameOf is the name of an anonymousAction or an authoredAction.
func nameOf(a sim.Action[*toyState]) string {
	switch a := a.(type) {
	case anonymousAction:
		return a.name
	case *authoredAction:
		return a.name
	case *typedAction:
		return a.name
	}
	return ""
}

// toyCodecs are codecs with the toy state, its actions and its command registered.
func toyCodecs() (*sim.Codecs, error) {
	c := sim.NewCodecs()
	return c, errs.Join(
		c.RegisterState(decodeToyState),
		c.RegisterState(decodeOtherState),
		c.RegisterAction(decodeAnonymousAction),
		c.RegisterAction(decodeAddAction),
		c.RegisterAction(decodeOtherAction),
		c.RegisterCommand(decodeAddCommand),
		c.RegisterOnFail(testDrop{}, decodeTestDrop),
		c.RegisterOnFail(testCompensate{}, decodeTestCompensate),
	)
}

// testDrop and testCompensate are the kinds of OnFail of these tests: the real ones are in package
// onfail, which imports this package.
type testDrop struct{}

const (
	testDropID       sim.OnFailID = 60001
	testCompensateID sim.OnFailID = 60002
)

func (testDrop) ID() sim.OnFailID                                           { return testDropID }
func (testDrop) Handle(b sim.Bouncer, _ sim.Message, _ sim.DeliveryFailure) { b.Drop() }
func (f testDrop) Clone() sim.OnFail                                        { return f }
func (testDrop) Encode(*binary.Writer) error                                { return nil }

func decodeTestDrop(
	*binary.Reader, codec.Version, sim.MessageDecoder,
) (sim.OnFail, error) {
	return testDrop{}, nil
}

type testCompensate struct{ With sim.Message }

func (testCompensate) ID() sim.OnFailID { return testCompensateID }
func (f testCompensate) Handle(b sim.Bouncer, _ sim.Message,
	_ sim.DeliveryFailure) {
	b.Resend(f.With)
}
func (f testCompensate) Clone() sim.OnFail {
	f.With = f.With.Clone()
	return f
}
func (f testCompensate) Nested() sim.Message { return f.With }
func (f testCompensate) Encode(w *binary.Writer) error {
	if f.With == nil {
		return sim.ErrNoCodec
	}
	return f.With.Encode(w)
}

func decodeTestCompensate(
	r *binary.Reader, v codec.Version, nested sim.MessageDecoder,
) (sim.OnFail, error) {
	m, err := nested(r, v)
	return testCompensate{With: m}, err
}
