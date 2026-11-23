package replay

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// A toy state with a few actions and commands, for the tests of the log: no game is needed to write
// records and read them back.

// the toy state is of the type 0, so a plain number is the ID of a toy state
const toyType sim.StateType = 0

type toyState struct{ id sim.StateID }

func (s *toyState) ID() sim.StateID               { return s.id }
func (*toyState) Type() sim.StateType             { return toyType }
func (*toyState) String() string                  { return "toy" }
func (*toyState) Tick() uint64                    { return 0 }
func (*toyState) SetTick(uint64)                  {}
func (s *toyState) Clone() sim.State              { return s }
func (*toyState) Hash() uint64                    { return 0 }
func (*toyState) TakeEvents() []sim.Event         { return nil }
func (s *toyState) Encode(w *binary.Writer) error { w.Uvarint(uint64(s.id)); return nil }
func decodeToyState(r *binary.Reader, _ codec.Version) (*toyState, error) {
	return &toyState{id: sim.StateID(r.Uvarint())}, r.Err()
}

type toyMoveAction struct {
	CharacterID uint64
	Dir         uint8
	Ticks       uint32
}

func (*toyMoveAction) ID() sim.ActionID { return 60001 }
func (*toyMoveAction) Priority() int16  { return 0 }
func (*toyMoveAction) Apply(
	*sim.Context, *toyState, *sim.Registry[*toyState], *sim.Plan[*toyState],
) {
}
func (a *toyMoveAction) Encode(w *binary.Writer) {
	w.Uvarint(a.CharacterID)
	w.Uint8(a.Dir)
	w.Uvarint(uint64(a.Ticks))
}
func decodeToyMoveAction(
	r *binary.Reader, _ codec.Version,
) (sim.Action[*toyState], error) {
	a := &toyMoveAction{CharacterID: r.Uvarint(), Dir: r.Uint8(), Ticks: uint32(r.Uvarint())}
	return a, r.Err()
}

type toyChatAction struct {
	FromID uint64
	Text   string
}

func (*toyChatAction) ID() sim.ActionID { return 60002 }
func (*toyChatAction) Priority() int16  { return 0 }
func (*toyChatAction) Apply(
	*sim.Context, *toyState, *sim.Registry[*toyState], *sim.Plan[*toyState],
) {
}
func (a *toyChatAction) Encode(w *binary.Writer) { w.Uvarint(a.FromID); w.String(a.Text) }
func decodeToyChatAction(
	r *binary.Reader, _ codec.Version,
) (sim.Action[*toyState], error) {
	a := &toyChatAction{FromID: r.Uvarint(), Text: r.String()}
	return a, r.Err()
}

type toyRegenAction struct{}

func (*toyRegenAction) ID() sim.ActionID { return 60003 }
func (*toyRegenAction) Priority() int16  { return 0 }
func (*toyRegenAction) Apply(
	*sim.Context, *toyState, *sim.Registry[*toyState], *sim.Plan[*toyState],
) {
}
func (*toyRegenAction) Encode(*binary.Writer) {}
func decodeToyRegenAction(
	_ *binary.Reader, _ codec.Version,
) (sim.Action[*toyState], error) {
	return &toyRegenAction{}, nil
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
	return []sim.Action[*toyState]{&toyMoveAction{c.CharacterID, c.Dir, c.Ticks}}
}
func (c toyMove) Clone() sim.Command[*toyState] { return c }
func decodeToyMove(r *binary.Reader, _ codec.Version) (sim.Command[*toyState], error) {
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
	return []sim.Action[*toyState]{&toyChatAction{c.FromID, c.Text}}
}
func (c toyChat) Clone() sim.Command[*toyState] { return c }
func decodeToyChat(r *binary.Reader, _ codec.Version) (sim.Command[*toyState], error) {
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
func decodeToyEnter(r *binary.Reader, _ codec.Version) (sim.Command[*toyState], error) {
	c := toyEnter{Name: r.String(), Cooldown: r.Uvarint()}
	return c, r.Err()
}

func toyCodecs() *sim.Codecs {
	c := sim.NewCodecs()
	err := sim.Errors(
		c.RegisterState[*toyState](&toyState{}, decodeToyState),
		c.RegisterAction[*toyState](&toyMoveAction{}, decodeToyMoveAction),
		c.RegisterAction[*toyState](&toyChatAction{}, decodeToyChatAction),
		c.RegisterAction[*toyState](&toyRegenAction{}, decodeToyRegenAction),
		c.RegisterCommand[*toyState](toyMove{}, decodeToyMove),
		c.RegisterCommand[*toyState](toyChat{}, decodeToyChat),
		c.RegisterCommand[*toyState](toyEnter{}, decodeToyEnter),
		onfail.Register(c),
	)
	if err != nil {
		panic(err)
	}
	return c
}
