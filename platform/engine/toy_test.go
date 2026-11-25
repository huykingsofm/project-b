package engine

import (
	"github.com/huykingsofm/project-b/platform/config"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/errs"
)

// A toy state and action, for the tests of the mechanism: no game is needed to try an engine.

// the toy state is of the type 0, so a plain number is the ID of a toy state
const toyType sim.StateType = 0

func init() {
	for t, name := range map[sim.StateType]string{toyType: "toy", toyType + 1: "other"} {
		sim.MapStateTypeToName(t, name)
	}
}

type toyState struct {
	id    sim.StateID
	tick  uint64
	Total int64
}

func (s *toyState) ID() sim.StateID   { return s.id }
func (*toyState) Type() sim.StateType { return toyType }
func (s *toyState) Tick() uint64      { return s.tick }
func (s *toyState) NextTick()         { s.tick++ }
func (s *toyState) Clone() sim.State  { c := *s; return &c }

func (s *toyState) TakeEvents() []sim.Event { return nil }
func (s *toyState) Hash() uint64 {
	h := sim.NewHasher()
	h.U64(uint64(s.id))
	h.U64(uint64(s.Total))
	return h.Sum()
}
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

const toyActionID sim.ActionID = 60001

// toyAction adds N to the total of the toy state, and has an author.
type toyAction struct {
	N      int64
	Author int
}

func (toyAction) ID() sim.ActionID { return toyActionID }
func (toyAction) Priority() int16  { return 0 }
func (a toyAction) Actor() any     { return a.Author }
func (a toyAction) Apply(
	_ *sim.Context, s *toyState, _ *sim.Registry[*toyState], _ *sim.Plan[*toyState],
) {
	s.Total += a.N
}
func (a toyAction) Clone() sim.Action[*toyState] { return a }
func (a toyAction) Encode(w *binary.Writer)      { w.Varint(a.N); w.Varint(int64(a.Author)) }

func decodeToyAction(r *binary.Reader, _ codec.Version) (toyAction, error) {
	a := toyAction{N: r.Varint(), Author: int(r.Varint())}
	return a, r.Err()
}

// toyCommand asks for an add.
type toyCommand struct{ N int64 }

const toyCommandID sim.CommandID = 60001

func (toyCommand) ID() sim.CommandID { return toyCommandID }
func (c toyCommand) Encode(w *binary.Writer) error {
	w.Varint(c.N)
	return nil
}
func (c toyCommand) Translate() []sim.Action[*toyState] {
	return []sim.Action[*toyState]{toyAction{N: c.N}}
}
func (c toyCommand) Clone() sim.Command[*toyState] { return c }

func decodeToyCommand(
	r *binary.Reader, _ codec.Version,
) (toyCommand, error) {
	c := toyCommand{N: r.Varint()}
	return c, r.Err()
}

// forwardAction sends an add to another engine, and sets itself again when Again is more than 0.
type forwardAction struct {
	To    sim.StateID
	N     int64
	Again uint64
}

const forwardActionID sim.ActionID = 60002

func (forwardAction) ID() sim.ActionID { return forwardActionID }
func (forwardAction) Priority() int16  { return 0 }
func (forwardAction) Actor() any       { return nil }
func (a forwardAction) Apply(
	c *sim.Context, _ *toyState, r *sim.Registry[*toyState], _ *sim.Plan[*toyState],
) {
	r.Send(a.To, onfail.Retry{}, toyAction{N: a.N})
	if a.Again > 0 {
		r.ToSelfAt(c.Tick+a.Again, a)
	}
}
func (a forwardAction) Clone() sim.Action[*toyState] { return a }
func (a forwardAction) Encode(w *binary.Writer) {
	w.Uvarint(uint64(a.To))
	w.Varint(a.N)
	w.Uvarint(a.Again)
}

func decodeForwardAction(
	r *binary.Reader, _ codec.Version,
) (forwardAction, error) {
	a := forwardAction{To: sim.StateID(r.Uvarint()), N: r.Varint(), Again: r.Uvarint()}
	return a, r.Err()
}

// readAction copies the total of another engine, as that engine published it.
type readAction struct{ From sim.StateID }

const readActionID sim.ActionID = 60003

func (readAction) ID() sim.ActionID { return readActionID }
func (readAction) Priority() int16  { return 0 }
func (readAction) Actor() any       { return nil }
func (a readAction) Apply(
	_ *sim.Context, s *toyState, r *sim.Registry[*toyState], _ *sim.Plan[*toyState],
) {
	if other, err := r.Get[*toyState](a.From); err == nil {
		s.Total = other.Total
	}
}
func (a readAction) Clone() sim.Action[*toyState] { return a }
func (a readAction) Encode(w *binary.Writer)      { w.Uvarint(uint64(a.From)) }

func decodeReadAction(
	r *binary.Reader, _ codec.Version,
) (readAction, error) {
	a := readAction{From: sim.StateID(r.Uvarint())}
	return a, r.Err()
}

// newCluster makes a cluster with its configuration engine, and fails the test when it cannot.
func newCluster(t testing.TB, env *Env) *Cluster {
	t.Helper()
	c, err := NewCluster(env, 1, NoCheckpoints{}, config.NewStore())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// setConfig gives the store of the cluster the document, which the cluster sends to the
// configuration engine at its next step.
func setConfig(t testing.TB, c *Cluster, document map[string]string) {
	t.Helper()
	if err := c.ConfigStore().Apply(document); err != nil {
		t.Fatal(err)
	}
}

func toyCodecs() (*sim.Codecs, error) {
	c := sim.NewCodecs()
	return c, errs.Join(
		c.RegisterState(decodeToyState),
		c.RegisterAction(decodeToyAction),
		c.RegisterAction(decodeForwardAction),
		c.RegisterAction(decodeReadAction),
		c.RegisterCommand(decodeToyCommand),
		onfail.Register(c),
	)
}
