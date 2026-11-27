package replay_test

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/errs"
)

// A toy state with one action, for the tests of the recovery: no game is needed to write logs of
// engines that depend on one another and to read them back.

// the toy state is of the type 0, so a plain number is the ID of a toy state
const toyType sim.StateType = 0

func init() { sim.MapStateTypeToName(toyType, "toy") }

type toyState struct{}

func (*toyState) Type() sim.StateType           { return toyType }
func (*toyState) Tick() uint64                  { return 0 }
func (*toyState) NextTick()                     {}
func (s *toyState) Clone() sim.State            { return s }
func (*toyState) Hash() uint64                  { return 0 }
func (*toyState) TakeEvents() []sim.Event       { return nil }
func (s *toyState) Encode(*binary.Writer) error { return nil }
func decodeToyState(r *binary.Reader, _ codec.Version) (*toyState, error) {
	return &toyState{}, r.Err()
}

type toyAction struct{ N uint64 }

func (toyAction) ID() sim.ActionID { return 60001 }
func (toyAction) Priority() int16  { return 0 }
func (toyAction) Actor() any       { return nil }
func (toyAction) Apply(
	*sim.Context, *toyState, *sim.Registry[*toyState], *sim.Plan[*toyState],
) {
}
func (a toyAction) Clone() sim.Action[*toyState] { return a }
func (a toyAction) Encode(w *binary.Writer)      { w.Uvarint(a.N) }
func decodeToyAction(r *binary.Reader, _ codec.Version) (toyAction, error) {
	return toyAction{N: r.Uvarint()}, r.Err()
}

func toyCodecs() *sim.Codecs {
	c := sim.NewCodecs()
	err := errs.Join(
		c.RegisterState(decodeToyState),
		c.RegisterAction(decodeToyAction),
		onfail.Register(c),
	)
	if err != nil {
		panic(err)
	}
	return c
}

// aMessageFrom is a message that the engine from sent at its tick tick, with the number seq.
func aMessageFrom(from sim.StateID, seq, tick uint64) sim.Message {
	return sim.NewActionMessage[*toyState](3, onfail.Drop{}, toyAction{N: seq}).
		Stamp(sim.Header{From: from, Seq: seq, Tick: tick})
}
