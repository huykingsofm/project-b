package configengine

import (
	"fmt"
	"maps"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/stablemap"
)

func init() { sim.MapStateTypeToName(sim.ConfigStateType, "config") }

// Override sets one variable: its name and the text of its JSON value.
type Override struct {
	Name string
	Text string
}

// State is the state of the configuration engine: the overrides of the variables over their
// defaults, and the snapshot of them. The snapshot never changes, so a copy
// of the state shares it with the state it was made from.
type State struct {
	tick      uint64
	overrides stablemap.Map[string, string]
	snap      *config.Snapshot
}

var _ sim.State = (*State)(nil)
var _ sim.ConfigReader = (*State)(nil)

// NewState makes the state of a configuration that has every variable at its default, .
func NewState() *State { return &State{snap: config.Defaults()} }

// ID is the ID of the state: the configuration engine of a cluster is one.
func (*State) ID() sim.StateID { return sim.ConfigStateID }

// Type is the type of the state.
func (*State) Type() sim.StateType { return sim.ConfigStateType }

// Tick is the tick that the state is at.
func (s *State) Tick() uint64 { return s.tick }

// NextTick takes the state to the next tick.
func (s *State) NextTick() { s.tick++ }

// Clone returns a copy that shares nothing that can be changed with the state: the snapshot is
// shared, because it never changes.
func (s *State) Clone() sim.State {
	return &State{
		tick: s.tick, overrides: s.overrides.Clone(), snap: s.snap,
	}
}

// TakeEvents returns none: the configuration says nothing to the clients.
func (*State) TakeEvents() []sim.Event { return nil }

// Snapshot is the configuration, as a snapshot that no one changes: the only way to read it.
func (s *State) Snapshot() config.ReadOnlySnapshot { return s.snap }

// ApplyConfigChange sets the variables of set to the text of their value, and puts the variables
// of unset back to their default. When no variable ends with another value than it had, nothing
// changes; otherwise the snapshot is a new one. It returns an error, and
// changes nothing, when a value is wrong.
func (s *State) ApplyConfigChange(set []Override, unset []string) error {
	next := s.overrides.Clone()
	for _, o := range set {
		next.Set(o.Name, o.Text)
	}
	for _, name := range unset {
		next.Delete(name)
	}
	snap, _, err := config.Build(overridesOf(&next))
	if err != nil {
		return fmt.Errorf("the configuration is refused: %w", err)
	}
	if len(snap.Diff(s.snap)) == 0 {
		return nil
	}
	s.overrides, s.snap = next, snap
	return nil
}

// overridesOf is the overrides as the map that config.Build takes.
func overridesOf(m *stablemap.Map[string, string]) map[string]string {
	return maps.Collect(m.All())
}

// Encode writes the state: its tick and its overrides.
func (s *State) Encode(w *binary.Writer) error {
	w.Uvarint(s.tick)
	w.Uvarint(uint64(s.overrides.Len()))
	for name, text := range s.overrides.All() {
		w.String(name)
		w.String(text)
	}
	return nil
}

// Hash is the hash of the data of the state.
func (s *State) Hash() uint64 {
	h := sim.NewHasher()
	h.U64(s.tick)
	for name, text := range s.overrides.All() {
		h.String(name)
		h.String(text)
	}
	return h.Sum()
}

// DecodeState reads what Encode wrote, and makes the snapshot of the overrides again. It returns an
// error for bytes that are cut, or overrides that no snapshot can be made of.
func DecodeState(r *binary.Reader, _ codec.Version) (*State, error) {
	s := &State{tick: r.Uvarint()}
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, err
	}
	if n > uint64(r.Left()) { // an override takes at least two bytes
		return nil, fmt.Errorf("%w: %d overrides", binary.ErrShort, n)
	}
	for range n {
		name, text := r.String(), r.String()
		if err := r.Err(); err != nil {
			return nil, err
		}
		s.overrides.Set(name, text)
	}
	snap, _, err := config.Build(overridesOf(&s.overrides))
	if err != nil {
		return nil, fmt.Errorf("the overrides of the configuration: %w", err)
	}
	s.snap = snap
	return s, nil
}
