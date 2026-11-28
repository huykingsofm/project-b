package sim

import (
	"fmt"
	"maps"
	"math"
	"slices"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/logging"
)

// CoordinatorStateType is the type of the state of the coordinator engine, the last of the types.
const CoordinatorStateType StateType = math.MaxUint8

// CoordinatorStateID is the ID of the state of the coordinator engine, which every cluster has
// once: the engines read the configuration from the copy it publishes.
var CoordinatorStateID = NewStateID(CoordinatorStateType, 1)

// The IDs of the command and the action of the coordinator engine: the last of each register,
// as its state is the last of the types, so the game's own numbers, which count up from 1, never
// meet them.
const (
	UpdateConfigCommandID CommandID = math.MaxUint32
	UpdateConfigActionID  ActionID  = math.MaxUint32
	CreateEngineCommandID CommandID = math.MaxUint32 - 1
	CreateEngineActionID  ActionID  = math.MaxUint32 - 1
	RetireEngineCommandID CommandID = math.MaxUint32 - 2
	RetireEngineActionID  ActionID  = math.MaxUint32 - 2
)

func init() { MapStateTypeToName(CoordinatorStateType, "coordinator") }

// ConfigOverride sets one variable: its name and the text of its JSON value.
type ConfigOverride struct {
	Name string
	Text string
}

// CoordinatorState is the state of the coordinator engine: its tick, and the snapshot of the
// configuration, which holds the text of every variable that was set. The snapshot never changes,
// so a copy of the state shares it with the state it was made from.
type CoordinatorState struct {
	tick uint64
	snap *config.Snapshot
}

// NewCoordinatorState makes the state of a configuration that has every variable at its default.
func NewCoordinatorState() *CoordinatorState { return &CoordinatorState{snap: config.Defaults()} }

// DecodeCoordinatorState reads what Encode wrote. It returns an error for bytes that are cut, or
// texts that no snapshot can be made of.
func DecodeCoordinatorState(r *binary.Reader, _ codec.Version) (*CoordinatorState, error) {
	tick := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, err
	}
	snap, err := config.DecodeSnapshot(r)
	if err != nil {
		return nil, fmt.Errorf("the snapshot of the configuration: %w", err)
	}
	return &CoordinatorState{tick: tick, snap: snap}, nil
}

// Type is the type of the state.
func (*CoordinatorState) Type() StateType { return CoordinatorStateType }

// Tick is the tick that the state is at.
func (s *CoordinatorState) Tick() uint64 { return s.tick }

// AdvanceTick takes the state to the next tick.
func (s *CoordinatorState) AdvanceTick() { s.tick++ }

// Clone returns a copy that shares nothing that can be changed with the state: the snapshot is
// shared, because it never changes.
func (s *CoordinatorState) Clone() State { return &CoordinatorState{tick: s.tick, snap: s.snap} }

// TakeEvents returns none: the configuration says nothing to the clients.
func (*CoordinatorState) TakeEvents() []Event { return nil }

// Snapshot is the configuration, as a snapshot that no one changes: the only way to read it.
func (s *CoordinatorState) Snapshot() config.ReadOnlySnapshot { return s.snap }

// ApplyConfigChange sets the variables of set to the text of their value, and puts the variables
// of unset back to their default (see config.Snapshot.Apply). It returns an error, and changes
// nothing, when a value is wrong.
func (s *CoordinatorState) ApplyConfigChange(set []ConfigOverride, unset []string) error {
	texts := make(map[string]string, len(set))
	for _, o := range set {
		texts[o.Name] = o.Text
	}
	snap, err := s.snap.Apply(texts, unset)
	if err != nil {
		return fmt.Errorf("the configuration is refused: %w", err)
	}
	s.snap = snap
	return nil
}

// Encode writes the state: its tick and its snapshot.
func (s *CoordinatorState) Encode(w *binary.Writer) error {
	w.Uvarint(s.tick)
	s.snap.Encode(w)
	return nil
}

// Hash is the hash of the data of the state.
func (s *CoordinatorState) Hash() uint64 {
	h := NewHasher()
	h.U64(s.tick)
	for name, text := range s.snap.Texts() {
		h.String(name)
		h.String(text)
	}
	return h.Sum()
}

// HandleDirective takes the response of the cluster to a request of the coordinator engine, which
// an admin asked for by a command. The engine that was made is found in the cluster; a request that
// was refused is logged, for the admin to see.
func (s *CoordinatorState) HandleDirective(
	_ *Context, _ *ActionRegistry[*CoordinatorState], response DirectiveResponse,
) {
	if response.Failure != "" {
		logging.Error(
			"the cluster refused a request of the coordinator engine",
			"kind", response.Kind, "engine", response.Engine, "failure", response.Failure,
		)
	}
}

var _ State = (*CoordinatorState)(nil)

// UpdateConfigCommand is what a config service sends the coordinator engine when the
// configuration changed: the variables that now have another value, with the text of it, and the
// variables that went back to their default.
type UpdateConfigCommand struct {
	Set   []ConfigOverride
	Unset []string
}

// NewUpdateConfigCommand makes the command that takes the configuration current has to the wanted
// one: the variables of wanted that have another value, set to their text, and the ones that went
// back to their default, unset. It says false, with no command, when no variable would change.
func NewUpdateConfigCommand(
	current config.ReadOnlySnapshot, wanted *config.Snapshot,
) (UpdateConfigCommand, bool) {
	texts := maps.Collect(wanted.Texts())
	var c UpdateConfigCommand
	for _, name := range wanted.Diff(current) {
		if text, ok := texts[name]; ok {
			c.Set = append(c.Set, ConfigOverride{Name: name, Text: text})
		} else {
			c.Unset = append(c.Unset, name)
		}
	}
	return c, len(c.Set)+len(c.Unset) > 0
}

func decodeUpdateConfigCommand(r *binary.Reader, _ codec.Version) (UpdateConfigCommand, error) {
	set, unset, err := decodeChange(r)
	return UpdateConfigCommand{Set: set, Unset: unset}, err
}

func (UpdateConfigCommand) ID() CommandID { return UpdateConfigCommandID }

// Encode writes the fields of the command.
func (c UpdateConfigCommand) Encode(w *binary.Writer) error {
	encodeChange(w, c.Set, c.Unset)
	return nil
}

// Translate makes the action that does what the command says.
func (c UpdateConfigCommand) Translate() []Action[*CoordinatorState] {
	return []Action[*CoordinatorState]{UpdateConfigAction{Set: c.Set, Unset: c.Unset}}
}

// Clone returns a copy that shares neither list with the command.
func (c UpdateConfigCommand) Clone() Command[*CoordinatorState] {
	return UpdateConfigCommand{Set: slices.Clone(c.Set), Unset: slices.Clone(c.Unset)}
}

// UpdateConfigAction sets the variables of Set and puts the variables of Unset back to their
// default (see CoordinatorState.ApplyConfigChange). An action does not fail: a change that the
// configuration refuses is logged and changes nothing.
type UpdateConfigAction struct {
	Set   []ConfigOverride
	Unset []string
}

func decodeUpdateConfigAction(r *binary.Reader, _ codec.Version) (UpdateConfigAction, error) {
	set, unset, err := decodeChange(r)
	return UpdateConfigAction{Set: set, Unset: unset}, err
}

func (UpdateConfigAction) ID() ActionID { return UpdateConfigActionID }

func (UpdateConfigAction) Priority() int16 { return 0 }

func (UpdateConfigAction) Actor() any { return nil }

// Clone returns a copy that shares neither list with the action.
func (a UpdateConfigAction) Clone() Action[*CoordinatorState] {
	return UpdateConfigAction{Set: slices.Clone(a.Set), Unset: slices.Clone(a.Unset)}
}

// Apply changes the configuration.
func (a UpdateConfigAction) Apply(
	_ *Context, s *CoordinatorState, _ *ActionRegistry[*CoordinatorState],
	_ *Plan[*CoordinatorState],
) {
	if err := s.ApplyConfigChange(a.Set, a.Unset); err != nil {
		logging.Warn("the coordinator engine refused a change", "err", err)
	}
}

// Encode writes the fields of the action.
func (a UpdateConfigAction) Encode(w *binary.Writer) { encodeChange(w, a.Set, a.Unset) }

// CreateEngineAction asks the cluster for an engine of the state, which runs the actions at its
// tick 2: the cluster gives it its ID. The coordinator engine is the only engine that is not made
// by another, so this is how an admin starts the engines that nothing else starts.
type CreateEngineAction struct {
	State State
	Start []BaseAction
	Tag   []byte
}

func decodeCreateEngineAction(
	c *Codecs, r *binary.Reader, version codec.Version,
) (CreateEngineAction, error) {
	a := CreateEngineAction{Tag: copyOf(r.Bytes())}
	if err := r.Err(); err != nil {
		return CreateEngineAction{}, err
	}
	var err error
	if a.State, err = c.DecodeState(r, version); err != nil {
		return CreateEngineAction{}, err
	}
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return CreateEngineAction{}, err
	}
	if n > uint64(r.Left()) { // an action takes at least one byte
		return CreateEngineAction{}, fmt.Errorf("%w: %d start actions", binary.ErrShort, n)
	}
	for range n {
		start, err := c.DecodeAction(r, version)
		if err != nil {
			return CreateEngineAction{}, err
		}
		a.Start = append(a.Start, start)
	}
	return a, r.Err()
}

func (CreateEngineAction) ID() ActionID { return CreateEngineActionID }

func (CreateEngineAction) Priority() int16 { return 0 }

func (CreateEngineAction) Actor() any { return nil }

// Clone returns the action itself: Apply copies the state the engine is made from.
func (a CreateEngineAction) Clone() Action[*CoordinatorState] { return a }

// Apply puts the request for the engine in the registry.
func (a CreateEngineAction) Apply(
	_ *Context, _ *CoordinatorState, r *ActionRegistry[*CoordinatorState],
	_ *Plan[*CoordinatorState],
) {
	if a.State == nil {
		logging.Error("the coordinator engine was asked for an engine with no state")
		return
	}
	r.RequestCreateEngineOf(a.Tag, a.State.Clone(), slices.Clone(a.Start))
}

// Encode writes the tag and the engine: not its ID, which EncodeAction writes first.
func (a CreateEngineAction) Encode(w *binary.Writer) {
	w.Bytes(a.Tag)
	if err := EncodeState(w, a.State); err != nil {
		logging.Error("an action that asks for an engine could not be written", "err", err)
		return
	}
	w.Uvarint(uint64(len(a.Start)))
	for _, start := range a.Start {
		if err := EncodeAction(w, start); err != nil {
			logging.Error("an action that asks for an engine could not be written", "err", err)
			return
		}
	}
}

// RetireEngineAction asks the cluster to end the engine with the ID. The coordinator engine is
// never ended: every engine reads the configuration from it.
type RetireEngineAction struct{ Target StateID }

func decodeRetireEngineAction(r *binary.Reader, _ codec.Version) (RetireEngineAction, error) {
	return RetireEngineAction{Target: r.Uint[StateID]()}, r.Err()
}

func (RetireEngineAction) ID() ActionID { return RetireEngineActionID }

func (RetireEngineAction) Priority() int16 { return 0 }

func (RetireEngineAction) Actor() any { return nil }

// Clone returns the action itself: it holds only values.
func (a RetireEngineAction) Clone() Action[*CoordinatorState] { return a }

// Apply puts the request to end the engine in the registry.
func (a RetireEngineAction) Apply(
	_ *Context, _ *CoordinatorState, r *ActionRegistry[*CoordinatorState],
	_ *Plan[*CoordinatorState],
) {
	r.RequestRetireEngine(a.Target, nil)
}

// Encode writes the ID of the engine.
func (a RetireEngineAction) Encode(w *binary.Writer) { w.Uvarint(uint64(a.Target)) }

// CreateEngineCommand is what the code that runs the cluster sends the coordinator engine to have
// an engine made: the same as CreateEngineAction, from outside.
type CreateEngineCommand struct {
	State State
	Start []BaseAction
	Tag   []byte
}

func decodeCreateEngineCommand(
	c *Codecs, r *binary.Reader, version codec.Version,
) (CreateEngineCommand, error) {
	a, err := decodeCreateEngineAction(c, r, version)
	return CreateEngineCommand(a), err
}

func (CreateEngineCommand) ID() CommandID { return CreateEngineCommandID }

// Encode writes the tag and the engine.
func (c CreateEngineCommand) Encode(w *binary.Writer) error {
	CreateEngineAction(c).Encode(w)
	return nil
}

// Translate makes the action that asks for the engine.
func (c CreateEngineCommand) Translate() []Action[*CoordinatorState] {
	return []Action[*CoordinatorState]{CreateEngineAction(c)}
}

// Clone returns the command itself: the action copies the state it makes the engine from.
func (c CreateEngineCommand) Clone() Command[*CoordinatorState] { return c }

// RetireEngineCommand is what the code that runs the cluster sends the coordinator engine to have
// an engine ended.
type RetireEngineCommand struct{ Target StateID }

func decodeRetireEngineCommand(r *binary.Reader, _ codec.Version) (RetireEngineCommand, error) {
	return RetireEngineCommand{Target: r.Uint[StateID]()}, r.Err()
}

func (RetireEngineCommand) ID() CommandID { return RetireEngineCommandID }

// Encode writes the ID of the engine.
func (c RetireEngineCommand) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(c.Target))
	return nil
}

// Translate makes the action that asks for the engine to end.
func (c RetireEngineCommand) Translate() []Action[*CoordinatorState] {
	return []Action[*CoordinatorState]{RetireEngineAction(c)}
}

// Clone returns the command itself: it holds only values.
func (c RetireEngineCommand) Clone() Command[*CoordinatorState] { return c }

// encodeChange writes what an action and a command of the configuration both hold.
func encodeChange(w *binary.Writer, set []ConfigOverride, unset []string) {
	w.Uvarint(uint64(len(set)))
	for _, o := range set {
		w.String(o.Name)
		w.String(o.Text)
	}
	w.Uvarint(uint64(len(unset)))
	for _, name := range unset {
		w.String(name)
	}
}

// decodeChange reads what encodeChange wrote.
func decodeChange(r *binary.Reader) ([]ConfigOverride, []string, error) {
	var set []ConfigOverride
	var unset []string
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, nil, err
	}
	if n > uint64(r.Left()) { // an override takes at least two bytes
		return nil, nil, fmt.Errorf("%w: %d overrides", binary.ErrShort, n)
	}
	for range n {
		set = append(set, ConfigOverride{Name: r.String(), Text: r.String()})
	}
	m := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, nil, err
	}
	if m > uint64(r.Left()) { // a name takes at least one byte
		return nil, nil, fmt.Errorf("%w: %d names", binary.ErrShort, m)
	}
	for range m {
		unset = append(unset, r.String())
	}
	return set, unset, r.Err()
}
