package sim

import (
	"bytes"
	"fmt"
	"math"
	"slices"

	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/logging"
	"github.com/huykingsofm/project-b/shared/stablemap"
)

// CoordinatorStateType is the type of the state of the coordinator engine, the one before the last
// of the types (the last is the one of the configuration engine).
const CoordinatorStateType StateType = math.MaxUint8 - 1

// CoordinatorStateID is the ID of the state of the coordinator engine, which every cluster has
// once: it gives the IDs of the other engines and makes and ends them. It is the first of the
// engines in the graph of what depends on what: the log of each engine it makes begins with a
// record that depends on its tick, and so does the record that ends one.
var CoordinatorStateID = NewStateID(CoordinatorStateType, 1)

// The IDs of the actions and the commands of the coordinator: the last of each register but the
// ones of the configuration, so the game's own numbers, which count up from 1, never meet them.
const (
	CreateEngineActionID  ActionID  = math.MaxUint32 - 1
	RetireEngineActionID  ActionID  = math.MaxUint32 - 2
	CreateEngineCommandID CommandID = math.MaxUint32 - 1
	RetireEngineCommandID CommandID = math.MaxUint32 - 2
)

func init() { MapStateTypeToName(CoordinatorStateType, "coordinator") }

// CoordinatorState is the state of the coordinator engine: its tick, and the highest number that it
// gave for each type of state. An ID is not given twice, so it names one thing, in a log, in a
// checkpoint and in a message that waits; but what a crash took with its tick is given again, along
// with the engines that depended on that tick. A request that is sent again is taken once: the
// engine that takes a message drops a copy by its number.
type CoordinatorState struct {
	tick  uint64
	given stablemap.Map[StateType, uint64]
}

var _ State = (*CoordinatorState)(nil)

// NewCoordinatorState makes the state of a coordinator that gave no ID.
func NewCoordinatorState() *CoordinatorState { return &CoordinatorState{} }

// Type is the type of the state.
func (*CoordinatorState) Type() StateType { return CoordinatorStateType }

// Tick is the tick that the state is at.
func (s *CoordinatorState) Tick() uint64 { return s.tick }

// NextTick takes the state to the next tick.
func (s *CoordinatorState) NextTick() { s.tick++ }

// Clone returns a copy that shares nothing with the state.
func (s *CoordinatorState) Clone() State {
	return &CoordinatorState{tick: s.tick, given: s.given.Clone()}
}

// TakeEvents returns none: the coordinator says nothing to the clients.
func (*CoordinatorState) TakeEvents() []Event { return nil }

// NextEngineID gives the next ID of a type of state: the numbers of a type are counted up from 1.
func (s *CoordinatorState) NextEngineID(t StateType) StateID {
	n, _ := s.given.Get(t)
	n++
	s.given.Set(t, n)
	return NewStateID(t, n)
}

// Encode writes the state: its tick and the highest number given for each type.
func (s *CoordinatorState) Encode(w *binary.Writer) error {
	w.Uvarint(s.tick)
	w.Uvarint(uint64(s.given.Len()))
	for t, n := range s.given.All() {
		w.Uvarint(uint64(t))
		w.Uvarint(n)
	}
	return nil
}

// Hash is the hash of the data of the state.
func (s *CoordinatorState) Hash() uint64 {
	h := NewHasher()
	h.U64(s.tick)
	for t, n := range s.given.All() {
		h.U64(uint64(t))
		h.U64(n)
	}
	return h.Sum()
}

// DecodeCoordinatorState reads what Encode wrote.
func DecodeCoordinatorState(r *binary.Reader, _ codec.Version) (*CoordinatorState, error) {
	s := &CoordinatorState{tick: r.Uvarint()}
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, err
	}
	if n > uint64(r.Left()) { // a number given takes at least two bytes
		return nil, fmt.Errorf("%w: %d types", binary.ErrShort, n)
	}
	for range n {
		t, given := StateType(r.Uvarint()), r.Uvarint()
		if err := r.Err(); err != nil {
			return nil, err
		}
		s.given.Set(t, given)
	}
	return s, r.Err()
}

// EngineCreation is an engine that a tick of the coordinator made the ID of, and that the engine
// of the coordinator makes when the tick is done: the state is as the first tick of the new engine
// finds it, and the actions run once at that tick. Ticket is the number that the code that runs
// the cluster gave the request, to find out the ID (0 for a request of another engine).
type EngineCreation struct {
	ID     StateID
	State  State
	Start  []BaseAction
	Ticket uint64
}

// EngineRetirement is an engine that a tick of the coordinator asked to end, which it does between
// two of its ticks.
type EngineRetirement struct{ Target StateID }

// CreateEngineAction asks the coordinator for an engine: it gives it the next ID of its type, and
// the engine of the coordinator makes it when the tick is done. The engine that is made lists
// itself where it must be listed, by an action that is among the ones it starts with, since only it
// knows its ID by then (R28).
type CreateEngineAction struct {
	State  State
	Start  []BaseAction
	Ticket uint64
	// payload is the state and the start actions as bytes, which the action holds so that it is
	// written the same every time and a failure to write it is known when it is made
	payload []byte
}

// NewCreateEngineAction makes the action that asks for an engine of the state, which runs the
// actions at its first tick. It is an error when the state or an action cannot be written.
func NewCreateEngineAction(
	st State, start []BaseAction, ticket uint64,
) (CreateEngineAction, error) {
	payload, err := encodeCreation(st, start)
	if err != nil {
		return CreateEngineAction{}, err
	}
	return CreateEngineAction{State: st, Start: start, Ticket: ticket, payload: payload}, nil
}

func (CreateEngineAction) ID() ActionID    { return CreateEngineActionID }
func (CreateEngineAction) Priority() int16 { return 0 }
func (CreateEngineAction) Actor() any      { return nil }

// Clone returns the action itself: Apply copies the state it makes the engine from.
func (a CreateEngineAction) Clone() Action[*CoordinatorState] { return a }

// Apply gives the next ID of the type of the state, and puts the engine to be made in the registry.
func (a CreateEngineAction) Apply(
	_ *Context, s *CoordinatorState, r *Registry[*CoordinatorState], _ *Plan[*CoordinatorState],
) {
	if a.State == nil {
		logging.Error("the coordinator was asked for an engine with no state")
		return
	}
	r.CreateEngine(EngineCreation{
		ID: s.NextEngineID(a.State.Type()), State: a.State.Clone(), Start: slices.Clone(a.Start),
		Ticket: a.Ticket,
	})
}

// Encode writes the ticket and the engine: not its ID, which EncodeAction writes first.
func (a CreateEngineAction) Encode(w *binary.Writer) {
	payload := a.payload
	if payload == nil {
		var err error
		if payload, err = encodeCreation(a.State, a.Start); err != nil {
			logging.Error("an action that asks for an engine could not be written", "err", err)
		}
	}
	w.Uvarint(a.Ticket)
	w.Bytes(payload)
}

func decodeCreateEngineAction(
	c *Codecs, r *binary.Reader, version codec.Version,
) (CreateEngineAction, error) {
	ticket, payload := r.Uvarint(), copyOf(r.Bytes())
	if err := r.Err(); err != nil {
		return CreateEngineAction{}, err
	}
	st, start, err := decodeCreation(c, payload, version)
	if err != nil {
		return CreateEngineAction{}, err
	}
	return CreateEngineAction{State: st, Start: start, Ticket: ticket, payload: payload}, nil
}

// RetireEngineAction asks the coordinator to end the engine with the ID, which retires between two
// of its ticks. The coordinator and the configuration engine are never ended: every engine reads
// the configuration, and nothing makes the others.
type RetireEngineAction struct{ Target StateID }

func (RetireEngineAction) ID() ActionID    { return RetireEngineActionID }
func (RetireEngineAction) Priority() int16 { return 0 }
func (RetireEngineAction) Actor() any      { return nil }

// Clone returns the action itself: it holds only values.
func (a RetireEngineAction) Clone() Action[*CoordinatorState] { return a }

// Apply puts the engine to be ended in the registry.
func (a RetireEngineAction) Apply(
	_ *Context, _ *CoordinatorState, r *Registry[*CoordinatorState], _ *Plan[*CoordinatorState],
) {
	if a.Target == ConfigStateID || a.Target == CoordinatorStateID {
		logging.Warn("the coordinator was asked to end an engine it never ends", "id", a.Target)
		return
	}
	r.RetireEngine(EngineRetirement{Target: a.Target})
}

// Encode writes the ID of the engine.
func (a RetireEngineAction) Encode(w *binary.Writer) { w.Uvarint(uint64(a.Target)) }

func decodeRetireEngineAction(r *binary.Reader, _ codec.Version) (RetireEngineAction, error) {
	a := RetireEngineAction{Target: r.Uint[StateID]()}
	return a, r.Err()
}

// CreateEngineCommand is what the code that runs the cluster sends the coordinator to have an
// engine made: the same as CreateEngineAction, from outside.
type CreateEngineCommand struct {
	State   State
	Start   []BaseAction
	Ticket  uint64
	payload []byte
}

// NewCreateEngineCommand makes the command that asks for an engine of the state, which runs the
// actions at its first tick. It is an error when the state or an action cannot be written.
func NewCreateEngineCommand(
	st State, start []BaseAction, ticket uint64,
) (CreateEngineCommand, error) {
	payload, err := encodeCreation(st, start)
	if err != nil {
		return CreateEngineCommand{}, err
	}
	return CreateEngineCommand{State: st, Start: start, Ticket: ticket, payload: payload}, nil
}

func (CreateEngineCommand) ID() CommandID { return CreateEngineCommandID }

// Encode writes the ticket and the engine.
func (c CreateEngineCommand) Encode(w *binary.Writer) error {
	payload := c.payload
	if payload == nil {
		var err error
		if payload, err = encodeCreation(c.State, c.Start); err != nil {
			return err
		}
	}
	w.Uvarint(c.Ticket)
	w.Bytes(payload)
	return nil
}

// Translate makes the action that asks for the engine.
func (c CreateEngineCommand) Translate() []Action[*CoordinatorState] {
	return []Action[*CoordinatorState]{
		CreateEngineAction{State: c.State, Start: c.Start, Ticket: c.Ticket, payload: c.payload},
	}
}

// Clone returns the command itself: the engine copies the state it makes the engine from.
func (c CreateEngineCommand) Clone() Command[*CoordinatorState] { return c }

func decodeCreateEngineCommand(
	c *Codecs, r *binary.Reader, version codec.Version,
) (CreateEngineCommand, error) {
	a, err := decodeCreateEngineAction(c, r, version)
	return CreateEngineCommand{
		State: a.State, Start: a.Start, Ticket: a.Ticket, payload: a.payload,
	}, err
}

// RetireEngineCommand is what the code that runs the cluster sends the coordinator to have an
// engine ended.
type RetireEngineCommand struct{ Target StateID }

func (RetireEngineCommand) ID() CommandID { return RetireEngineCommandID }

// Encode writes the ID of the engine.
func (c RetireEngineCommand) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(c.Target))
	return nil
}

// Translate makes the action that asks for the engine to end.
func (c RetireEngineCommand) Translate() []Action[*CoordinatorState] {
	return []Action[*CoordinatorState]{RetireEngineAction{Target: c.Target}}
}

// Clone returns the command itself: it holds only values.
func (c RetireEngineCommand) Clone() Command[*CoordinatorState] { return c }

func decodeRetireEngineCommand(r *binary.Reader, _ codec.Version) (RetireEngineCommand, error) {
	c := RetireEngineCommand{Target: r.Uint[StateID]()}
	return c, r.Err()
}

// encodeCreation writes the state and the actions that an engine is made from.
func encodeCreation(st State, start []BaseAction) ([]byte, error) {
	var w binary.Writer
	if err := EncodeState(&w, st); err != nil {
		return nil, err
	}
	w.Uvarint(uint64(len(start)))
	for _, a := range start {
		if err := EncodeAction(&w, a); err != nil {
			return nil, err
		}
	}
	return bytes.Clone(w.B), nil
}

// decodeCreation reads what encodeCreation wrote.
func decodeCreation(
	c *Codecs, payload []byte, version codec.Version,
) (State, []BaseAction, error) {
	r := binary.NewReader(payload)
	st, err := c.DecodeState(r, version)
	if err != nil {
		return nil, nil, err
	}
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, nil, err
	}
	if n > uint64(r.Left()) { // an action takes at least one byte
		return nil, nil, fmt.Errorf("%w: %d start actions", binary.ErrShort, n)
	}
	var start []BaseAction
	for range n {
		a, err := c.DecodeAction(r, version)
		if err != nil {
			return nil, nil, err
		}
		start = append(start, a)
	}
	return st, start, r.Err()
}
