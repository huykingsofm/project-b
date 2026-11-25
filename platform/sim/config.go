package sim

import (
	"fmt"
	"math"
	"slices"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/logging"
)

// ConfigStateType is the type of the state of the configuration engine, the last of the types.
const ConfigStateType StateType = math.MaxUint8

// ConfigStateID is the ID of the state of the configuration engine, which every cluster has once:
// the engines read the configuration from the copy it publishes.
var ConfigStateID = NewStateID(ConfigStateType, 1)

// The IDs of the command and the action of the configuration engine: the last of each register,
// as its state is the last of the types, so the game's own numbers, which count up from 1, never
// meet them.
const (
	UpdateConfigCommandID CommandID = math.MaxUint32
	UpdateConfigActionID  ActionID  = math.MaxUint32
)

func init() { MapStateTypeToName(ConfigStateType, "config") }

// ConfigOverride sets one variable: its name and the text of its JSON value.
type ConfigOverride struct {
	Name string
	Text string
}

// ConfigState is the state of the configuration engine: its tick, and the snapshot of the
// configuration, which holds the text of every variable that was set. The snapshot never changes,
// so a copy of the state shares it with the state it was made from.
type ConfigState struct {
	tick uint64
	snap *config.Snapshot
}

var _ State = (*ConfigState)(nil)

// NewConfigState makes the state of a configuration that has every variable at its default.
func NewConfigState() *ConfigState { return &ConfigState{snap: config.Defaults()} }

// ID is the ID of the state: the configuration engine of a cluster is one.
func (*ConfigState) ID() StateID { return ConfigStateID }

// Type is the type of the state.
func (*ConfigState) Type() StateType { return ConfigStateType }

// Tick is the tick that the state is at.
func (s *ConfigState) Tick() uint64 { return s.tick }

// NextTick takes the state to the next tick.
func (s *ConfigState) NextTick() { s.tick++ }

// Clone returns a copy that shares nothing that can be changed with the state: the snapshot is
// shared, because it never changes.
func (s *ConfigState) Clone() State { return &ConfigState{tick: s.tick, snap: s.snap} }

// TakeEvents returns none: the configuration says nothing to the clients.
func (*ConfigState) TakeEvents() []Event { return nil }

// Snapshot is the configuration, as a snapshot that no one changes: the only way to read it.
func (s *ConfigState) Snapshot() config.ReadOnlySnapshot { return s.snap }

// ApplyConfigChange sets the variables of set to the text of their value, and puts the variables
// of unset back to their default (see config.Snapshot.Apply). It returns an error, and changes
// nothing, when a value is wrong.
func (s *ConfigState) ApplyConfigChange(set []ConfigOverride, unset []string) error {
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
func (s *ConfigState) Encode(w *binary.Writer) error {
	w.Uvarint(s.tick)
	s.snap.Encode(w)
	return nil
}

// Hash is the hash of the data of the state.
func (s *ConfigState) Hash() uint64 {
	h := NewHasher()
	h.U64(s.tick)
	for name, text := range s.snap.Texts() {
		h.String(name)
		h.String(text)
	}
	return h.Sum()
}

// DecodeConfigState reads what Encode wrote. It returns an error for bytes that are cut, or texts
// that no snapshot can be made of.
func DecodeConfigState(r *binary.Reader, _ codec.Version) (*ConfigState, error) {
	tick := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, err
	}
	snap, err := config.DecodeSnapshot(r)
	if err != nil {
		return nil, fmt.Errorf("the snapshot of the configuration: %w", err)
	}
	return &ConfigState{tick: tick, snap: snap}, nil
}

// UpdateConfigCommand is what a config service sends the configuration engine when the
// configuration changed: the variables that now have another value, with the text of it, and the
// variables that went back to their default.
type UpdateConfigCommand struct {
	Set   []ConfigOverride
	Unset []string
}

// NewUpdateConfigCommand makes the command that takes the configuration current has to the one the
// document says: the text of the JSON value of each variable that it sets, by name; the others are
// at their default. It says false, with no command, when no variable would change. It returns an
// error, with no command, when the document is not a configuration: the names that no variable has
// are left out, as config.Build does.
func NewUpdateConfigCommand(
	current config.ReadOnlySnapshot, document map[string]string,
) (UpdateConfigCommand, bool, error) {
	wanted, err := config.Build(document)
	if err != nil {
		return UpdateConfigCommand{}, false, err
	}
	var c UpdateConfigCommand
	for _, name := range wanted.Diff(current) {
		if text, ok := document[name]; ok {
			c.Set = append(c.Set, ConfigOverride{Name: name, Text: text})
		} else {
			c.Unset = append(c.Unset, name)
		}
	}
	return c, len(c.Set)+len(c.Unset) > 0, nil
}

func (UpdateConfigCommand) ID() CommandID { return UpdateConfigCommandID }

// Encode writes the fields of the command.
func (c UpdateConfigCommand) Encode(w *binary.Writer) error {
	encodeChange(w, c.Set, c.Unset)
	return nil
}

// Translate makes the action that does what the command says.
func (c UpdateConfigCommand) Translate() []Action[*ConfigState] {
	return []Action[*ConfigState]{UpdateConfigAction{Set: c.Set, Unset: c.Unset}}
}

// Clone returns a copy that shares neither list with the command.
func (c UpdateConfigCommand) Clone() Command[*ConfigState] {
	return UpdateConfigCommand{Set: slices.Clone(c.Set), Unset: slices.Clone(c.Unset)}
}

func decodeUpdateConfigCommand(r *binary.Reader, _ codec.Version) (UpdateConfigCommand, error) {
	set, unset, err := decodeChange(r)
	return UpdateConfigCommand{Set: set, Unset: unset}, err
}

// UpdateConfigAction sets the variables of Set and puts the variables of Unset back to their
// default (see ConfigState.ApplyConfigChange). An action does not fail: a change that the
// configuration refuses is logged and changes nothing.
type UpdateConfigAction struct {
	Set   []ConfigOverride
	Unset []string
}

func (UpdateConfigAction) ID() ActionID    { return UpdateConfigActionID }
func (UpdateConfigAction) Priority() int16 { return 0 }
func (UpdateConfigAction) Actor() any      { return nil }

// Clone returns a copy that shares neither list with the action.
func (a UpdateConfigAction) Clone() Action[*ConfigState] {
	return UpdateConfigAction{Set: slices.Clone(a.Set), Unset: slices.Clone(a.Unset)}
}

// Apply changes the configuration.
func (a UpdateConfigAction) Apply(
	_ *Context, s *ConfigState, _ *Registry[*ConfigState], _ *Plan[*ConfigState],
) {
	if err := s.ApplyConfigChange(a.Set, a.Unset); err != nil {
		logging.Warn("the configuration engine refused a change", "err", err)
	}
}

// Encode writes the fields of the action.
func (a UpdateConfigAction) Encode(w *binary.Writer) { encodeChange(w, a.Set, a.Unset) }

func decodeUpdateConfigAction(r *binary.Reader, _ codec.Version) (UpdateConfigAction, error) {
	set, unset, err := decodeChange(r)
	return UpdateConfigAction{Set: set, Unset: unset}, err
}

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
