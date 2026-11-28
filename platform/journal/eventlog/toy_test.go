package eventlog_test

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/errs"
)

// A toy state with a few actions and commands, for the tests of the log: no game is needed to write
// records and read them back.

// the toy state is of the type 0, so a plain number is the ID of a toy state
const toyType sim.StateType = 0

func init() { sim.MapStateTypeToName(toyType, "toy") }

type toyState struct{}

func (*toyState) Type() sim.StateType           { return toyType }
func (*toyState) Tick() uint64                  { return 0 }
func (*toyState) AdvanceTick()                  {}
func (s *toyState) Clone() sim.State            { return s }
func (*toyState) Hash() uint64                  { return 0 }
func (*toyState) TakeEvents() []sim.Event       { return nil }
func (s *toyState) Encode(*binary.Writer) error { return nil }
func decodeToyState(r *binary.Reader, _ codec.Version) (*toyState, error) {
	return &toyState{}, r.Err()
}

type toyMoveAction struct {
	CharacterID uint64
	Dir         uint8
	Ticks       uint32
}

func (toyMoveAction) ID() sim.ActionID { return 60001 }
func (toyMoveAction) Priority() int16  { return 0 }
func (toyMoveAction) Actor() any       { return nil }
func (toyMoveAction) Apply(
	*sim.Context, *toyState, *sim.ActionRegistry[*toyState], *sim.Plan[*toyState],
) {
}
func (a toyMoveAction) Clone() sim.Action[*toyState] { return a }
func (a toyMoveAction) Encode(w *binary.Writer) {
	w.Uvarint(a.CharacterID)
	w.Uint8(a.Dir)
	w.Uvarint(uint64(a.Ticks))
}
func decodeToyMoveAction(
	r *binary.Reader, _ codec.Version,
) (toyMoveAction, error) {
	a := toyMoveAction{CharacterID: r.Uvarint(), Dir: r.Uint8(), Ticks: uint32(r.Uvarint())}
	return a, r.Err()
}

type toyChatAction struct {
	FromID uint64
	Text   string
}

func (toyChatAction) ID() sim.ActionID { return 60002 }
func (toyChatAction) Priority() int16  { return 0 }
func (toyChatAction) Actor() any       { return nil }
func (toyChatAction) Apply(
	*sim.Context, *toyState, *sim.ActionRegistry[*toyState], *sim.Plan[*toyState],
) {
}
func (a toyChatAction) Clone() sim.Action[*toyState] { return a }
func (a toyChatAction) Encode(w *binary.Writer)      { w.Uvarint(a.FromID); w.String(a.Text) }
func decodeToyChatAction(
	r *binary.Reader, _ codec.Version,
) (toyChatAction, error) {
	a := toyChatAction{FromID: r.Uvarint(), Text: r.String()}
	return a, r.Err()
}

type toyRegenAction struct{}

func (toyRegenAction) ID() sim.ActionID { return 60003 }
func (toyRegenAction) Priority() int16  { return 0 }
func (toyRegenAction) Actor() any       { return nil }
func (toyRegenAction) Apply(
	*sim.Context, *toyState, *sim.ActionRegistry[*toyState], *sim.Plan[*toyState],
) {
}
func (a toyRegenAction) Clone() sim.Action[*toyState] { return a }
func (toyRegenAction) Encode(*binary.Writer)          {}
func decodeToyRegenAction(
	_ *binary.Reader, _ codec.Version,
) (toyRegenAction, error) {
	return toyRegenAction{}, nil
}

// toyMove, toyChat and toyEnter are the commands of the tests.
type toyMove struct {
	CharacterID uint64
	Dir         uint8
	Ticks       uint32
}

func (toyMove) ID() sim.CommandID { return 60001 }
func (c toyMove) Encode(w *binary.Writer) error {
	w.Uvarint(c.CharacterID)
	w.Uint8(c.Dir)
	w.Uvarint(uint64(c.Ticks))
	return nil
}
func (c toyMove) Translate() []sim.Action[*toyState] {
	return []sim.Action[*toyState]{toyMoveAction{c.CharacterID, c.Dir, c.Ticks}}
}
func (c toyMove) Clone() sim.Command[*toyState] { return c }
func decodeToyMove(r *binary.Reader, _ codec.Version) (toyMove, error) {
	c := toyMove{CharacterID: r.Uvarint(), Dir: r.Uint8(), Ticks: uint32(r.Uvarint())}
	return c, r.Err()
}

type toyChat struct {
	FromID uint64
	Text   string
}

func (toyChat) ID() sim.CommandID { return 60002 }
func (c toyChat) Encode(w *binary.Writer) error {
	w.Uvarint(c.FromID)
	w.String(c.Text)
	return nil
}
func (c toyChat) Translate() []sim.Action[*toyState] {
	return []sim.Action[*toyState]{toyChatAction{c.FromID, c.Text}}
}
func (c toyChat) Clone() sim.Command[*toyState] { return c }
func decodeToyChat(r *binary.Reader, _ codec.Version) (toyChat, error) {
	c := toyChat{FromID: r.Uvarint(), Text: r.String()}
	return c, r.Err()
}

type toyEnter struct {
	Name     string
	Cooldown uint64
}

func (toyEnter) ID() sim.CommandID { return 60003 }
func (c toyEnter) Encode(w *binary.Writer) error {
	w.String(c.Name)
	w.Uvarint(c.Cooldown)
	return nil
}
func (c toyEnter) Translate() []sim.Action[*toyState] { return nil }
func (c toyEnter) Clone() sim.Command[*toyState]      { return c }
func decodeToyEnter(r *binary.Reader, _ codec.Version) (toyEnter, error) {
	c := toyEnter{Name: r.String(), Cooldown: r.Uvarint()}
	return c, r.Err()
}

func toyCodecs() *sim.Codecs {
	c := sim.NewCodecs()
	err := errs.Join(
		c.RegisterState(decodeToyState),
		c.RegisterAction(decodeToyMoveAction),
		c.RegisterAction(decodeToyChatAction),
		c.RegisterAction(decodeToyRegenAction),
		c.RegisterCommand(decodeToyMove),
		c.RegisterCommand(decodeToyChat),
		c.RegisterCommand(decodeToyEnter),
		onfail.Register(c),
	)
	if err != nil {
		panic(err)
	}
	return c
}
