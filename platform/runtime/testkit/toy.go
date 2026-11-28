package testkit

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/runtime/cluster"
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/runtime/router"
	"github.com/huykingsofm/project-b/platform/runtime/stable"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/errs"
)

// A toy state and action, for the tests of the mechanism: no game is needed to try an engine.

// the toy state is of the type 0, so a plain number is the ID of a toy state
const ToyType sim.StateType = 0

func init() {
	for t, name := range map[sim.StateType]string{
		ToyType: "toy", ToyType + 1: "other", ToyType + 2: "stuck",
	} {
		sim.MapStateTypeToName(t, name)
	}
}

const ToyActionID sim.ActionID = 60001

const ToyCommandID sim.CommandID = 60001

const ForwardActionID sim.ActionID = 60002

const ReadActionID sim.ActionID = 60003

// newCluster makes a cluster with its coordinator engine, and fails the test when it cannot.
// AddToys makes n toy engines in the cluster, which gives them the numbers that follow the ones it
// has given, and returns them in that order.
func AddToys(t testing.TB, c *cluster.Cluster, n int) []*engine.Engine[*ToyState] {
	t.Helper()
	var engines []*engine.Engine[*ToyState]
	for range n {
		engines = append(engines, AddEngineOf(t, c, &ToyState{}))
	}
	return engines
}

// AddEngineOf puts an engine of the state in the cluster at the tick of the state, with the next
// number that its orchestrator gives: the engine is made in an environment of its own and made
// again in the cluster from its checkpoint, as a recovery does, so no request made it.
func AddEngineOf[S sim.State](t testing.TB, c *cluster.Cluster, st S) *engine.Engine[S] {
	t.Helper()
	id, err := c.Orchestrator().NextStateID(st.Type())
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := engine.NewEngine(st, id, NewToyEnv(t)).EncodeCheckpoint()
	if err != nil {
		t.Fatal(err)
	}
	handle, err := c.AddEngineByCheckpoint(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	return handle.(*engine.Engine[S])
}

func NewCluster(t testing.TB, env *engine.Environment) *cluster.Cluster {
	t.Helper()
	orchestrator, err := cluster.NewOrchestrator("", config.NewStore())
	if err != nil {
		t.Fatal(err)
	}
	c, err := cluster.NewCluster(env, orchestrator, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// SetConfig gives the store of the cluster the document, which the cluster sends to the
// coordinator engine at its next step.
func SetConfig(t testing.TB, c *cluster.Cluster, document map[string]string) {
	t.Helper()
	if err := c.ConfigStore().Apply(document); err != nil {
		t.Fatal(err)
	}
}

func ToyCodecs() (*sim.Codecs, *engine.EngineRegistry, error) {
	c, engines := sim.NewCodecs(), engine.NewEngineRegistry()
	return c, engines, errs.Join(
		engines.Register(c, DecodeToyState),
		c.RegisterAction(DecodeToyAction),
		c.RegisterAction(DecodeForwardAction),
		c.RegisterAction(DecodeReadAction),
		c.RegisterCommand(DecodeToyCommand),
		onfail.Register(c),
	)
}

// NewToyEnv makes the environment of a cluster of toy engines.
func NewToyEnv(t testing.TB) *engine.Environment {
	t.Helper()
	codecs, engines, err := ToyCodecs()
	if err != nil {
		t.Fatal(err)
	}
	return &engine.Environment{
		Router: router.NewRouter(), EventLog: &eventlog.MemLog{},
		StableStore: stable.NewStateStore(), Codecs: codecs, EngineRegistry: engines, Seed: 1,
		QueueSize:       16,
		Emitter:         engine.NopEmitter{},
		AckBoard:        stable.NewAckBoard(),
		DirectiveRouter: router.NewDirectiveRouter(),
	}
}

// NewToyCluster is a cluster with n toy engines, which the cluster gives the numbers 1 to n.
func NewToyCluster(t testing.TB, n int) (*cluster.Cluster, []*engine.Engine[*ToyState]) {
	t.Helper()
	c := NewCluster(t, NewToyEnv(t))
	return c, AddToys(t, c, n)
}

// ToyMessage is a message from the engine 1 to the engine with the number, sent at its tick seq.
func ToyMessage(to sim.StateID, seq uint64) sim.Message {
	return sim.NewActionMessage(to, onfail.Drop{}, ToyAction{N: 1}).
		Stamp(sim.Header{From: 1, Seq: seq, Tick: seq})
}

type ToyState struct {
	tick  uint64
	Total int64
}

// NewToyStateAtTick is a toy state that is at the tick, for an engine that starts there.
func NewToyStateAtTick(tick uint64) *ToyState { return &ToyState{tick: tick} }

func DecodeToyState(r *binary.Reader, _ codec.Version) (*ToyState, error) {
	s := &ToyState{tick: r.Uvarint(), Total: r.Varint()}
	return s, r.Err()
}

func (*ToyState) Type() sim.StateType { return ToyType }

func (s *ToyState) Tick() uint64 { return s.tick }

func (s *ToyState) AdvanceTick() { s.tick++ }

func (s *ToyState) Clone() sim.State { c := *s; return &c }

func (s *ToyState) TakeEvents() []sim.Event { return nil }

// HandleDirective counts the answers of the cluster: 100 times the number of the engine that was
// made, or -1 when it was not made.
func (s *ToyState) HandleDirective(
	_ *sim.Context, _ *sim.ActionRegistry[*ToyState], d sim.DirectiveResponse,
) {
	if d.Failure != "" {
		s.Total--
		return
	}
	s.Total += int64(d.Engine.Local()) * 100
}

func (s *ToyState) Hash() uint64 {
	h := sim.NewHasher()
	h.U64(uint64(s.Total))
	return h.Sum()
}

func (s *ToyState) Encode(w *binary.Writer) error {
	w.Uvarint(s.tick)
	w.Varint(s.Total)
	return nil
}

var _ sim.DirectiveHandler[*ToyState] = (*ToyState)(nil)

// ToyAction adds N to the total of the toy state, and has an author.
type ToyAction struct {
	N      int64
	Author int
}

func DecodeToyAction(r *binary.Reader, _ codec.Version) (ToyAction, error) {
	a := ToyAction{N: r.Varint(), Author: int(r.Varint())}
	return a, r.Err()
}

func (ToyAction) ID() sim.ActionID { return ToyActionID }

func (ToyAction) Priority() int16 { return 0 }

func (a ToyAction) Actor() any { return a.Author }

func (a ToyAction) Apply(
	_ *sim.Context, s *ToyState, _ *sim.ActionRegistry[*ToyState], _ *sim.Plan[*ToyState],
) {
	s.Total += a.N
}

func (a ToyAction) Clone() sim.Action[*ToyState] { return a }

func (a ToyAction) Encode(w *binary.Writer) { w.Varint(a.N); w.Varint(int64(a.Author)) }

// ToyCommand asks for an add.
type ToyCommand struct{ N int64 }

func DecodeToyCommand(
	r *binary.Reader, _ codec.Version,
) (ToyCommand, error) {
	c := ToyCommand{N: r.Varint()}
	return c, r.Err()
}

func (ToyCommand) ID() sim.CommandID { return ToyCommandID }

func (c ToyCommand) Encode(w *binary.Writer) error {
	w.Varint(c.N)
	return nil
}

func (c ToyCommand) Translate() []sim.Action[*ToyState] {
	return []sim.Action[*ToyState]{ToyAction{N: c.N}}
}

func (c ToyCommand) Clone() sim.Command[*ToyState] { return c }

// ForwardAction sends an add to another engine, and sets itself again when Again is more than 0.
type ForwardAction struct {
	To    sim.StateID
	N     int64
	Again uint64
}

func DecodeForwardAction(
	r *binary.Reader, _ codec.Version,
) (ForwardAction, error) {
	a := ForwardAction{To: sim.StateID(r.Uvarint()), N: r.Varint(), Again: r.Uvarint()}
	return a, r.Err()
}

func (ForwardAction) ID() sim.ActionID { return ForwardActionID }

func (ForwardAction) Priority() int16 { return 0 }

func (ForwardAction) Actor() any { return nil }

func (a ForwardAction) Apply(
	c *sim.Context, _ *ToyState, r *sim.ActionRegistry[*ToyState], _ *sim.Plan[*ToyState],
) {
	r.Send(a.To, onfail.Retry{}, ToyAction{N: a.N})
	if a.Again > 0 {
		r.ToSelfAt(c.Tick+a.Again, a)
	}
}

func (a ForwardAction) Clone() sim.Action[*ToyState] { return a }

func (a ForwardAction) Encode(w *binary.Writer) {
	w.Uvarint(uint64(a.To))
	w.Varint(a.N)
	w.Uvarint(a.Again)
}

// ReadAction copies the total of another engine, as that engine published it.
type ReadAction struct{ From sim.StateID }

func DecodeReadAction(
	r *binary.Reader, _ codec.Version,
) (ReadAction, error) {
	a := ReadAction{From: sim.StateID(r.Uvarint())}
	return a, r.Err()
}

func (ReadAction) ID() sim.ActionID { return ReadActionID }

func (ReadAction) Priority() int16 { return 0 }

func (ReadAction) Actor() any { return nil }

func (a ReadAction) Apply(
	_ *sim.Context, s *ToyState, r *sim.ActionRegistry[*ToyState], _ *sim.Plan[*ToyState],
) {
	if other, err := r.Get[*ToyState](a.From); err == nil {
		s.Total = other.Total
	}
}

func (a ReadAction) Clone() sim.Action[*ToyState] { return a }

func (a ReadAction) Encode(w *binary.Writer) { w.Uvarint(uint64(a.From)) }
